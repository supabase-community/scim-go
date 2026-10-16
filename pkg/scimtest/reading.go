package scimtest

import (
	"mime"
	"net/http"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

type reading struct {
	*resource
}

func (r reading) run(t *testing.T) {
	t.Helper()

	t.Run("returns the resource", r.checkGet)
	t.Run("returns 404 for an unknown id", r.checkUnknown)
	t.Run("returns the SCIM media type", r.checkMediaType)
	t.Run("never returns a returned:never attribute", r.checkNever)
}

func (r reading) checkGet(t *testing.T) {
	_, created := r.create(t, newUnique())

	res := r.client().Get(t, r.path(created))
	expectStatus(t, res, http.StatusOK, "RFC 7644 3.4.1")
	if got := idOf(Decode[core.Object](t, res)); got != idOf(created) {
		t.Errorf("scimtest: GET %s (RFC 7644 3.4.1): id %q, want %q", r.path(created), got, idOf(created))
	}
	r.expectLocated(t, res)
}

func (r reading) checkUnknown(t *testing.T) {
	RequireError(t, r.client().Get(t, r.kind.Endpoint+"/scimtest-"+newUnique()), http.StatusNotFound, "")
}

func (r reading) checkNever(t *testing.T) {
	attribute := r.attribute(t, "writable returned:never attribute", func(a *core.Attribute) bool {
		return a.Returned == core.ReturnedNever && a.Mutability != core.MutabilityReadOnly && a.Type == core.TypeString && !a.MultiValued
	})
	unique := newUnique()
	body := r.body(t, unique)
	body.Set(attribute.Name, "Scimtest-"+unique+"!1")
	res, created := r.post(t, body)
	if created == nil {
		t.Skipf("n/a: the server rejected the %s value", attribute.Name)
	}

	read := r.client().Get(t, r.path(created))
	expectStatus(t, read, http.StatusOK, "RFC 7644 3.4.1")
	for _, object := range []core.Object{created, Decode[core.Object](t, read)} {
		if object.Has(attribute.Name) {
			t.Errorf("scimtest: %s %s (RFC 7643 7): returned the returned:never attribute %s", res.Request.Method, r.kind.Endpoint, attribute.Name)
		}
	}
}

// RFC 7644 Section 3.8: the format defaults to "application/scim+json".
func (r reading) checkMediaType(t *testing.T) {
	r.suite.should(t, "RFC 7644 3.8 responses default to application/scim+json")
	res, created := r.create(t, newUnique())

	for _, res := range []*http.Response{res, r.client().Get(t, r.path(created)), r.client().Get(t, r.kind.Endpoint)} {
		if mediaType, _, _ := mime.ParseMediaType(res.Header.Get("Content-Type")); mediaType != protocol.MediaType {
			t.Errorf("scimtest: %s %s (RFC 7644 3.8): Content-Type %q, want %q", res.Request.Method, res.Request.URL.Path, mediaType, protocol.MediaType)
		}
	}
}
