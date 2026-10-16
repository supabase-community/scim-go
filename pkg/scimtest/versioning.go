package scimtest

import (
	"net/http"
	"testing"

	"github.com/supabase-community/scim-go/pkg/patch"
)

type versioning struct {
	*resource
}

func (r versioning) run(t *testing.T) {
	t.Helper()

	supports(t, "etag", r.config.ETag.Supported)
	t.Run("returns an ETag on create and read", r.checkETag)
	t.Run("returns 304 for a current If-None-Match", r.checkIfNoneMatch)
	t.Run("accepts a current If-Match and changes the version", r.checkIfMatch)
	t.Run("rejects a stale If-Match", r.checkStaleIfMatch)
}

func (r versioning) checkETag(t *testing.T) {
	res, created := r.create(t, newUnique())
	etag := r.etag(t, res)
	read := r.client().Get(t, r.path(created))
	expectStatus(t, read, http.StatusOK, "RFC 7644 3.4.1")

	if got := r.etag(t, read); r.suite.strict && got != etag {
		t.Errorf("SHOULD: GET %s (RFC 7232 2.3): ETag %q, want %q from the create for an unchanged resource", r.path(created), got, etag)
	}
	if version := metaOf(created)["version"]; r.suite.strict && version != etag {
		t.Errorf("SHOULD: POST %s (RFC 7644 3.14): meta.version %v, want ETag %q", r.kind.Endpoint, version, etag)
	}
}

// RFC 7232 Section 3.2: a GET whose If-None-Match matches the current version is answered with 304.
func (r versioning) checkIfNoneMatch(t *testing.T) {
	res, created := r.create(t, newUnique())

	read := r.client().With(WithHeader("If-None-Match", r.etag(t, res))).Get(t, r.path(created))
	expectStatus(t, read, http.StatusNotModified, "RFC 7644 3.14")
}

func (r versioning) checkIfMatch(t *testing.T) {
	r.suite.should(t, "RFC 7644 3.14 a PUT with the current ETag in If-Match succeeds, though RFC 7232 3.1 strong comparison may reject a weak ETag")
	unique := newUnique()
	res, created := r.create(t, unique)
	etag := r.etag(t, res)
	body := r.body(t, unique)
	attribute := firstOf(r.schemas.Base().Attributes, patchable)
	if attribute != nil {
		body.Set(attribute.Name, placeholder+" "+newUnique())
	}

	replaced := r.client().With(WithHeader("If-Match", etag)).Put(t, r.path(created), body)
	expectStatus(t, replaced, http.StatusOK, "RFC 7644 3.14")
	if got := r.etag(t, replaced); attribute != nil && got == etag {
		t.Errorf("scimtest: PUT %s (RFC 7232 2.3): ETag %q did not change after %s changed", r.path(created), got, attribute.Name)
	}
}

// RFC 7644 Section 3.12, Table 8: 409 when the version number does not match, 412 when the resource has changed.
func (r versioning) checkStaleIfMatch(t *testing.T) {
	r.suite.should(t, "RFC 7644 3.12 Table 8 409 or 412 for an If-Match that does not match the current ETag")
	unique := newUnique()
	_, created := r.create(t, unique)
	stale := r.client().With(WithHeader("If-Match", `W/"scimtest-stale"`))

	expectStale(t, stale.Put(t, r.path(created), r.body(t, unique)), "RFC 7644 3.14")
	if attribute := firstOf(r.schemas.Base().Attributes, patchable); attribute != nil && r.config.Patch.Supported {
		expectStale(t, stale.Patch(t, r.path(created), ops(op(patch.OpReplace, attribute.Name, placeholder))), "RFC 7644 3.14")
	}
	expectStale(t, stale.Delete(t, r.path(created)), "RFC 7644 3.6")
	expectStatus(t, r.client().Get(t, r.path(created)), http.StatusOK, "RFC 7644 3.4.1")
}

func expectStale(t *testing.T, res *http.Response, section string) {
	t.Helper()

	if res.StatusCode != http.StatusConflict && res.StatusCode != http.StatusPreconditionFailed {
		t.Fatalf("scimtest: %s %s (%s, Table 8): status %d, want 409 or 412", res.Request.Method, res.Request.URL.RequestURI(), section, res.StatusCode)
	}
}

func (r versioning) etag(t *testing.T, res *http.Response) string {
	t.Helper()

	etag := res.Header.Get("ETag")
	if etag == "" {
		t.Fatalf("scimtest: %s %s (RFC 7644 3.14): no ETag header", res.Request.Method, res.Request.URL.Path)
	}
	return etag
}
