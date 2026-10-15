package scimtest

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

const (
	typeSub     = "type"
	valueSub    = "value"
	placeholder = "scimtest"
)

type resource struct {
	suite     *Suite
	config    *core.ServiceProviderConfig
	kind      core.ResourceType
	schemas   core.Schemas
	fixture   fixture
	transport transport
	searching int
}

func (r *resource) over(t *testing.T, tr transport) *resource {
	t.Helper()

	clone := *r
	clone.transport = tr
	if !tr.search {
		return &clone
	}
	if r.searching == 0 {
		r.searching = tr.send(t, r.client(), r.kind.Endpoint, url.Values{countParam: {"0"}}).StatusCode
	}
	if r.searching != http.StatusOK {
		r.suite.todo(t, "POST %s/.search returned %d", r.kind.Endpoint, r.searching)
	}
	return &clone
}

func (r *resource) checkCreate(t *testing.T) {
	t.Run("returns 201 with a Location", r.checkLocation)
	t.Run("ignores a readOnly id", r.checkReadOnly)
	t.Run("ignores other readOnly attributes", r.checkReadOnlyAttributes)
	t.Run("assigns meta and ignores the client's", r.checkMeta)
	t.Run("rejects a duplicate unique value", r.checkUniqueness)
	t.Run("rejects a missing required attribute", r.checkRequired)
}

func (r *resource) checkLocation(t *testing.T) {
	res, created := r.create(t, newUnique())
	location, _ := metaOf(created)["location"].(string)
	if got := res.Header.Get("Location"); got == "" || got != location {
		t.Errorf("scimtest: POST %s (RFC 7644 3.3): Location %q, want meta.location %q", r.kind.Endpoint, got, location)
	}
	r.expectLocated(t, res)
}

func (r *resource) checkReadOnly(t *testing.T) {
	body := r.body(t, newUnique())
	body.Set("id", "scimtest-readonly")
	if _, created := r.createBody(t, body); idOf(created) == "scimtest-readonly" {
		t.Errorf("scimtest: POST %s (RFC 7644 3.3): kept the readOnly id from the request", r.kind.Endpoint)
	}
}

func (r *resource) checkReadOnlyAttributes(t *testing.T) {
	attribute := r.attribute(t, "readOnly attribute", isReadOnly)
	marker := "scimtest-readonly-" + newUnique()
	body := r.body(t, newUnique())
	body.Set(attribute.Name, readOnlyValue(attribute, marker))

	_, created := r.createBody(t, body)
	expectIgnored(t, "POST "+r.kind.Endpoint+" (RFC 7644 3.3)", created.Get(attribute.Name), marker)
}

// RFC 7643 Section 3.1: "meta" SHALL be ignored when provided by clients.
func (r *resource) checkMeta(t *testing.T) {
	forged := map[string]any{"resourceType": placeholder, "created": "2000-01-01T00:00:00Z", "lastModified": "2000-01-01T00:00:00Z", "location": "https://scimtest.invalid/scimtest", "version": `W/"scimtest"`}
	body := r.body(t, newUnique())
	body.Set("meta", forged)

	_, created := r.createBody(t, body)
	meta := metaOf(created)
	for key, value := range forged {
		if meta[key] == value {
			t.Errorf("scimtest: POST %s (RFC 7643 3.1): kept the client's meta.%s %v", r.kind.Endpoint, key, value)
		}
	}
	if kind, ok := meta["resourceType"]; ok && kind != string(r.kind.Name) {
		t.Errorf("scimtest: POST %s (RFC 7643 3.1): meta.resourceType %v, want %s", r.kind.Endpoint, kind, r.kind.Name)
	}
	if meta["created"] != meta["lastModified"] {
		t.Errorf("scimtest: POST %s (RFC 7643 3.1): meta.lastModified %v, want meta.created %v", r.kind.Endpoint, meta["lastModified"], meta["created"])
	}
}

func (r *resource) checkUniqueness(t *testing.T) {
	unique := newUnique()
	body := r.body(t, unique)
	r.attribute(t, "unique attribute in its fixture", func(a *core.Attribute) bool { return isUnique(a) && body.Has(a.Name) })
	r.createBody(t, body)

	res, _ := r.post(t, r.body(t, unique))
	RequireError(t, res, http.StatusConflict, scimerrors.Uniqueness)
}

func (r *resource) checkRequired(t *testing.T) {
	r.suite.should(t, "RFC 7644 3.12 400 invalidValue for a missing required attribute")
	body := r.body(t, newUnique())
	body.Remove(r.attribute(t, "required attribute in its fixture", required(body)).Name)

	res, _ := r.post(t, body)
	expectStatus(t, res, http.StatusBadRequest, "RFC 7644 3.12")
}

func (r *resource) checkRead(t *testing.T) {
	t.Run("returns the resource", r.checkGet)
	t.Run("returns 404 for an unknown id", r.checkUnknown)
	t.Run("never returns a returned:never attribute", r.checkNever)
}

func (r *resource) checkGet(t *testing.T) {
	_, created := r.create(t, newUnique())

	res := r.client().Get(t, r.path(created))
	expectStatus(t, res, http.StatusOK, "RFC 7644 3.4.1")
	if got := idOf(Decode[core.Object](t, res)); got != idOf(created) {
		t.Errorf("scimtest: GET %s (RFC 7644 3.4.1): id %q, want %q", r.path(created), got, idOf(created))
	}
	r.expectLocated(t, res)
}

func (r *resource) checkUnknown(t *testing.T) {
	RequireError(t, r.client().Get(t, r.kind.Endpoint+"/scimtest-"+newUnique()), http.StatusNotFound, "")
}

func (r *resource) checkNever(t *testing.T) {
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

func (r *resource) checkList(t *testing.T) {
	t.Run("returns a ListResponse", func(t *testing.T) { r.list(t, url.Values{}) })
	t.Run("returns totalResults 0 when nothing matches", r.checkListEmpty)
	t.Run("locates every listed resource", r.checkListLocations)
}

func (r *resource) checkListEmpty(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	query := byID("scimtest-" + newUnique())

	if page := r.list(t, query); page.TotalResults != 0 || len(page.Resources) != 0 {
		t.Errorf("scimtest: %s (RFC 7644 3.4.2): totalResults %d with %d resources, want 0", r.transport.describe(r.kind.Endpoint, query), page.TotalResults, len(page.Resources))
	}
}

func (r *resource) checkListLocations(t *testing.T) {
	r.create(t, newUnique())
	query := url.Values{}

	for _, object := range r.list(t, query).Resources {
		if location, ok := metaOf(object)["location"].(string); ok && !strings.HasSuffix(location, r.path(object)) {
			t.Errorf("scimtest: %s (RFC 7643 3.1): meta.location %q, want the URI of %s", r.transport.describe(r.kind.Endpoint, query), location, r.path(object))
		}
	}
}

func (r *resource) checkFilter(t *testing.T) {
	t.Run("finds a resource by id", r.checkFilterID)
	t.Run("rejects an invalid filter", r.checkFilterInvalid)
	t.Run("rejects gt on a boolean", r.checkFilterBoolean)
	t.Run("operators", r.checkFilterOperators)
	t.Run("matches a value path within one value", r.checkFilterValuePath)
	t.Run("compares a boolean", r.checkFilterBooleanEquals)
	t.Run("compares meta attributes", r.checkFilterMeta)
	t.Run("binds and tighter than or", r.checkFilterPrecedence)
}

func (r *resource) checkFilterValuePath(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	attribute := r.attribute(t, "multi-valued attribute with a canonical type and a value", func(a *core.Attribute) bool {
		typed, value := a.SubAttributes.Lookup(typeSub), a.SubAttributes.Lookup(valueSub)
		return a.MultiValued && a.Mutability == core.MutabilityReadWrite && typed != nil && len(typed.CanonicalValues) > 1 && value != nil && isFilterable(value)
	})
	kinds := attribute.SubAttribute(typeSub).CanonicalValues[:2]
	unique := newUnique()
	alpha, beta := "scimtest alpha "+unique, "scimtest beta "+unique
	ids := make([]string, 0, 2)
	for _, values := range [][]string{{alpha, beta}, {beta, alpha}} {
		body := r.body(t, newUnique())
		body.Set(attribute.Name, []any{map[string]any{typeSub: kinds[0], valueSub: values[0]}, map[string]any{typeSub: kinds[1], valueSub: values[1]}})
		ids = append(ids, idOf(r.seed(t, body)))
	}
	filter := attribute.Name + "[type eq " + quoted(kinds[0]) + " and value eq " + quoted(alpha) + "]"

	r.expectMatches(t, filter, ids, ids[:1])
}

func (r *resource) checkFilterBooleanEquals(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	attribute := r.attribute(t, "readWrite singular boolean attribute", func(a *core.Attribute) bool {
		return a.Type == core.TypeBoolean && !a.MultiValued && a.Mutability == core.MutabilityReadWrite
	})
	ids := make([]string, 0, 2)
	for _, value := range []bool{true, false} {
		body := r.body(t, newUnique())
		body.Set(attribute.Name, value)
		ids = append(ids, idOf(r.seed(t, body)))
	}

	r.expectMatches(t, attribute.Name+" eq true", ids, ids[:1])
	r.expectMatches(t, attribute.Name+" eq false", ids, ids[1:])
	r.expectMatches(t, "not ("+attribute.Name+" eq true)", ids, ids[1:])
}

func (r *resource) checkFilterMeta(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	_, first := r.create(t, newUnique())
	_, second := r.create(t, newUnique())
	ids := []string{idOf(first), idOf(second)}
	created, _ := metaOf(first)["created"].(string)
	modified, _ := metaOf(first)["lastModified"].(string)

	r.expectMatches(t, "meta.resourceType eq "+quoted(string(r.kind.Name)), ids, ids)
	r.expectMatches(t, "meta.resourceType eq \"scimtest\"", ids, nil)
	if metaOf(second)["created"] == created || metaOf(second)["lastModified"] == modified {
		t.Skipf("n/a: both resources were created at %s", created)
	}
	r.expectMatches(t, "meta.lastModified gt "+quoted(modified), ids, ids[1:])
	r.expectMatches(t, "meta.created le "+quoted(created), ids, ids[:1])
}

// RFC 7644 Section 3.4.2.2: "not" takes precedence over "and", which takes precedence over "or".
func (r *resource) checkFilterPrecedence(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	ids := make([]string, 0, 3)
	for range 3 {
		_, created := r.create(t, newUnique())
		ids = append(ids, idOf(created))
	}

	r.expectMatches(t, idFilter(ids[0])+" or "+idFilter(ids[1])+" and "+idFilter(ids[2]), ids, ids[:1])
	r.expectMatches(t, "not ("+idFilter(ids[0])+") and "+idFilter(ids[1]), ids, ids[1:2])
}

func (r *resource) checkFilterBoolean(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	attribute := r.attribute(t, "singular boolean attribute", func(a *core.Attribute) bool { return a.Type == core.TypeBoolean && !a.MultiValued })
	RequireError(t, r.send(t, filterBy(attribute.Name+" gt true")), http.StatusBadRequest, scimerrors.InvalidFilter)
}

func (r *resource) checkFilterOperators(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	newFilterTable(t, r).run(t)
}

func (r *resource) checkFilterID(t *testing.T) {
	if r.suite.unsupported(t, "filter", r.config.Filter.Supported) {
		res := r.send(t, byID("scimtest-none"))
		expectStatus(t, res, http.StatusNotImplemented, "RFC 7644 3.12")
		return
	}
	_, created := r.create(t, newUnique())
	query := byID(idOf(created))

	if got := r.filtered(t, query); !slices.Equal(got, []string{idOf(created)}) {
		t.Errorf("scimtest: %s (RFC 7644 3.4.2.2): ids %v, want [%s]", r.transport.describe(r.kind.Endpoint, query), got, idOf(created))
	}
}

func (r *resource) checkFilterInvalid(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	r.suite.should(t, "RFC 7644 Table 9 400 invalidFilter for an invalid filter")
	RequireError(t, r.send(t, filterBy("id eq")), http.StatusBadRequest, scimerrors.InvalidFilter)
}

func (r *resource) checkSort(t *testing.T) {
	t.Run("accepts sortBy", r.checkSortBy)
	t.Run("sorts ascending by default and descending on request", r.checkSortOrder)
}

func (r *resource) checkSortBy(t *testing.T) {
	query := url.Values{"sortBy": {"id"}}
	if r.suite.unsupported(t, "sort", r.config.Sort.Supported) {
		expectStatus(t, r.send(t, query), http.StatusNotImplemented, "RFC 7644 3.12")
		return
	}
	r.list(t, query)
}

func (r *resource) checkSortOrder(t *testing.T) {
	supports(t, "sort", r.config.Sort.Supported)
	supports(t, "filter", r.config.Filter.Supported)
	attribute := r.attribute(t, "readWrite singular string attribute", patchable)
	unique := newUnique()
	ids := make([]string, 0, 2)
	for _, prefix := range []string{"scimtest a ", "scimtest b "} {
		body := r.body(t, newUnique())
		body.Set(attribute.Name, prefix+unique)
		_, created := r.createBody(t, body)
		ids = append(ids, idOf(created))
	}
	query := byID(ids...)
	query.Set("sortBy", attribute.Name)

	r.expectIDs(t, query, ids, "RFC 7644 3.4.2.3")
	query.Set("sortOrder", "ascending")
	r.expectIDs(t, query, ids, "RFC 7644 3.4.2.3")
	query.Set("sortOrder", "descending")
	r.expectIDs(t, query, []string{ids[1], ids[0]}, "RFC 7644 3.4.2.3")
}

func (r *resource) checkPagination(t *testing.T) {
	r.create(t, newUnique())
	r.create(t, newUnique())

	t.Run("returns only totalResults for count 0", func(t *testing.T) { r.expectPage(t, url.Values{countParam: {"0"}}, 0, 1) })
	t.Run("treats a negative count as 0", func(t *testing.T) { r.expectPage(t, url.Values{countParam: {"-1"}}, 0, 1) })
	t.Run("returns no more than count", func(t *testing.T) { r.expectPage(t, url.Values{countParam: {"1"}}, 1, 2) })
	t.Run("treats a startIndex below 1 as 1", func(t *testing.T) {
		query := url.Values{"startIndex": {"0"}, countParam: {"1"}}
		if page := r.list(t, query); page.StartIndex != 1 {
			t.Errorf("scimtest: %s (RFC 7644 3.4.2.4): startIndex %d, want 1", r.transport.describe(r.kind.Endpoint, query), page.StartIndex)
		}
	})
	t.Run("walks every result once, page by page", r.checkPageWalk)
	t.Run("returns no more than filter.maxResults", r.checkPageCap)
}

func (r *resource) checkPageWalk(t *testing.T) {
	supports(t, "sort", r.config.Sort.Supported)
	supports(t, "filter", r.config.Filter.Supported)
	ids := make([]string, 0, 3)
	for range 3 {
		_, created := r.create(t, newUnique())
		ids = append(ids, idOf(created))
	}
	query := byID(ids...)
	query.Set("sortBy", "id")
	query.Set(countParam, "3")
	want := idsOf(r.list(t, query).Resources)

	walked := make([]string, 0, len(ids))
	for start := range ids {
		page := maps.Clone(query)
		page.Set("startIndex", strconv.Itoa(start+1))
		page.Set(countParam, "1")
		walked = append(walked, idsOf(r.list(t, page).Resources)...)
	}
	if !slices.Equal(walked, want) {
		t.Errorf("scimtest: %s (RFC 7644 3.4.2.4): pages of 1 returned %v, want %v", r.transport.describe(r.kind.Endpoint, query), walked, want)
	}
}

func (r *resource) checkPageCap(t *testing.T) {
	if r.transport.search {
		t.Skip("n/a: checked once, over GET")
	}
	most := r.config.Filter.MaxResults
	if most <= 0 || most > 200 {
		t.Skipf("n/a: filter.maxResults is %d", most)
	}
	for total := r.list(t, url.Values{countParam: {"0"}}).TotalResults; total <= most; total++ {
		r.create(t, newUnique())
	}
	query := url.Values{countParam: {strconv.Itoa(most + 1)}}

	if page := r.list(t, query); len(page.Resources) > most {
		t.Errorf("scimtest: %s (RFC 7643 5): %d resources, want at most filter.maxResults %d", r.transport.describe(r.kind.Endpoint, query), len(page.Resources), most)
	}
}

func (r *resource) checkAttributes(t *testing.T) {
	unique := newUnique()
	body := r.body(t, unique)
	attribute := r.attribute(t, "returned:default attribute in its fixture", func(a *core.Attribute) bool {
		return a.Returned == core.ReturnedDefault && body.Has(a.Name)
	})
	_, created := r.createBody(t, body)

	t.Run("returns a requested attribute and id", func(t *testing.T) {
		r.expectAttributes(t, r.path(created)+"?attributes="+attribute.Name, []string{"id", attribute.Name}, nil)
	})
	t.Run("removes an excluded attribute", func(t *testing.T) {
		r.expectAttributes(t, r.path(created)+"?excludedAttributes="+attribute.Name, []string{"id"}, []string{attribute.Name})
	})
	t.Run("never excludes id", func(t *testing.T) {
		r.expectAttributes(t, r.path(created)+"?excludedAttributes=id", []string{"id"}, nil)
	})
}

func (r *resource) checkPatch(t *testing.T) {
	patching{r}.run(t)
}

func (r *resource) checkReplace(t *testing.T) {
	t.Run("replaces the resource", r.checkPut)
	t.Run("ignores readOnly attributes", r.checkPutReadOnly)
	t.Run("rejects a changed immutable attribute", r.checkImmutable)
}

func (r *resource) checkPut(t *testing.T) {
	unique := newUnique()
	_, created := r.create(t, unique)

	res := r.client().Put(t, r.path(created), r.body(t, unique))
	expectStatus(t, res, http.StatusOK, "RFC 7644 3.5.1")
	if got := idOf(Decode[core.Object](t, res)); got != idOf(created) {
		t.Errorf("scimtest: PUT %s (RFC 7644 3.5.1): id %q, want %q", r.path(created), got, idOf(created))
	}
	r.expectLocated(t, res)
}

func (r *resource) checkPutReadOnly(t *testing.T) {
	unique := newUnique()
	_, created := r.create(t, unique)
	marker := "scimtest-readonly-" + newUnique()
	body := r.body(t, unique)
	body.Set("id", marker)
	attribute := find(r.schemas.Base(), isReadOnly)
	if attribute != nil {
		body.Set(attribute.Name, readOnlyValue(attribute, marker))
	}

	res := r.client().Put(t, r.path(created), body)
	expectStatus(t, res, http.StatusOK, "RFC 7644 3.5.1")
	replaced := Decode[core.Object](t, res)
	if got := idOf(replaced); got != idOf(created) {
		t.Errorf("scimtest: PUT %s (RFC 7644 3.5.1): id %q, want %q", r.path(created), got, idOf(created))
	}
	if attribute != nil {
		expectIgnored(t, "PUT "+r.path(created)+" (RFC 7644 3.5.1)", replaced.Get(attribute.Name), marker)
	}
}

func (r *resource) checkImmutable(t *testing.T) {
	r.suite.should(t, "RFC 7644 3.5.1 400 mutability for a changed immutable attribute")
	body := r.body(t, newUnique())
	attribute := r.attribute(t, "immutable attribute in its fixture", func(a *core.Attribute) bool {
		return a.Mutability == core.MutabilityImmutable && a.Type == core.TypeString && !a.MultiValued && body.Has(a.Name)
	})
	_, created := r.createBody(t, body)
	body.Set(attribute.Name, "scimtest "+newUnique())

	RequireError(t, r.client().Put(t, r.path(created), body), http.StatusBadRequest, scimerrors.Mutability)
}

func (r *resource) checkDelete(t *testing.T) {
	t.Run("returns 404 for every operation afterwards", r.checkDeleted)
	t.Run("omits the resource from queries", r.checkDeletedUnlisted)
	t.Run("accepts a new resource with the same unique values", r.checkDeletedReused)
}

// RFC 7644 Section 3.6: the service provider MUST return 404 for all operations associated with a deleted resource.
func (r *resource) checkDeleted(t *testing.T) {
	unique := newUnique()
	_, created := r.create(t, unique)
	path := r.path(created)

	expectStatus(t, r.client().Delete(t, path), http.StatusNoContent, "RFC 7644 3.6")
	RequireError(t, r.client().Get(t, path), http.StatusNotFound, "")
	RequireError(t, r.client().Put(t, path, r.body(t, unique)), http.StatusNotFound, "")
	if attribute := find(r.schemas.Base(), patchable); attribute != nil && r.config.Patch.Supported {
		RequireError(t, r.client().Patch(t, path, ops(op(patch.OpReplace, attribute.Name, placeholder))), http.StatusNotFound, "")
	}
	RequireError(t, r.client().Delete(t, path), http.StatusNotFound, "")
}

func (r *resource) checkDeletedUnlisted(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	_, created := r.create(t, newUnique())
	expectStatus(t, r.client().Delete(t, r.path(created)), http.StatusNoContent, "RFC 7644 3.6")

	query := byID(idOf(created))
	if got := idsOf(r.list(t, query).Resources); len(got) != 0 {
		t.Errorf("scimtest: %s (RFC 7644 3.6): ids %v, want none", r.transport.describe(r.kind.Endpoint, query), got)
	}
}

func (r *resource) checkDeletedReused(t *testing.T) {
	r.suite.should(t, "RFC 7644 3.6 a deleted resource is not considered in conflict calculation")
	unique := newUnique()
	_, created := r.create(t, unique)
	expectStatus(t, r.client().Delete(t, r.path(created)), http.StatusNoContent, "RFC 7644 3.6")

	r.create(t, unique)
}

func (r *resource) checkVersioning(t *testing.T) {
	supports(t, "etag", r.config.ETag.Supported)
	t.Run("returns an ETag on create and read", r.checkETag)
	t.Run("returns 304 for a current If-None-Match", r.checkIfNoneMatch)
	t.Run("accepts a current If-Match and changes the version", r.checkIfMatch)
	t.Run("rejects a stale If-Match", r.checkStaleIfMatch)
}

func (r *resource) checkETag(t *testing.T) {
	res, created := r.create(t, newUnique())
	etag := r.etag(t, res)
	read := r.client().Get(t, r.path(created))
	expectStatus(t, read, http.StatusOK, "RFC 7644 3.4.1")

	if got := r.etag(t, read); got != etag {
		t.Errorf("scimtest: GET %s (RFC 7644 3.14): ETag %q, want %q from the create", r.path(created), got, etag)
	}
	if version := metaOf(created)["version"]; r.suite.strict && version != etag {
		t.Errorf("SHOULD: POST %s (RFC 7644 3.14): meta.version %v, want ETag %q", r.kind.Endpoint, version, etag)
	}
}

// RFC 7232 Section 3.2: a GET whose If-None-Match matches the current version is answered with 304.
func (r *resource) checkIfNoneMatch(t *testing.T) {
	res, created := r.create(t, newUnique())

	read := r.client().With(WithHeader("If-None-Match", r.etag(t, res))).Get(t, r.path(created))
	expectStatus(t, read, http.StatusNotModified, "RFC 7644 3.14")
}

func (r *resource) checkIfMatch(t *testing.T) {
	unique := newUnique()
	res, created := r.create(t, unique)
	etag := r.etag(t, res)
	body := r.body(t, unique)
	attribute := find(r.schemas.Base(), patchable)
	if attribute != nil {
		body.Set(attribute.Name, placeholder+" "+newUnique())
	}

	replaced := r.client().With(WithHeader("If-Match", etag)).Put(t, r.path(created), body)
	expectStatus(t, replaced, http.StatusOK, "RFC 7644 3.14")
	if got := r.etag(t, replaced); attribute != nil && got == etag {
		t.Errorf("scimtest: PUT %s (RFC 7644 3.14): ETag %q did not change after %s changed", r.path(created), got, attribute.Name)
	}
}

// RFC 7644 Section 3.12, Table 8: PUT, PATCH and DELETE fail with 412 when the resource has changed.
func (r *resource) checkStaleIfMatch(t *testing.T) {
	unique := newUnique()
	_, created := r.create(t, unique)
	stale := r.client().With(WithHeader("If-Match", `W/"scimtest-stale"`))

	expectStatus(t, stale.Put(t, r.path(created), r.body(t, unique)), http.StatusPreconditionFailed, "RFC 7232 3.1")
	if attribute := find(r.schemas.Base(), patchable); attribute != nil && r.config.Patch.Supported {
		expectStatus(t, stale.Patch(t, r.path(created), ops(op(patch.OpReplace, attribute.Name, placeholder))), http.StatusPreconditionFailed, "RFC 7232 3.1")
	}
	expectStatus(t, stale.Delete(t, r.path(created)), http.StatusPreconditionFailed, "RFC 7232 3.1")
	expectStatus(t, r.client().Get(t, r.path(created)), http.StatusOK, "RFC 7232 3.1")
}

func (r *resource) etag(t *testing.T, res *http.Response) string {
	t.Helper()

	etag := res.Header.Get("ETag")
	if etag == "" {
		t.Fatalf("scimtest: %s %s (RFC 7644 3.14): no ETag header", res.Request.Method, res.Request.URL.Path)
	}
	return etag
}

func (r *resource) attribute(t *testing.T, what string, match func(*core.Attribute) bool) *core.Attribute {
	t.Helper()

	attribute := find(r.schemas.Base(), match)
	if attribute == nil {
		t.Skipf("n/a: %s has no %s", r.kind.Name, what)
	}
	return attribute
}

func (r *resource) expectPatched(t *testing.T, res *http.Response, object core.Object) {
	t.Helper()

	if res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusOK {
		t.Fatalf("scimtest: PATCH %s (RFC 7644 3.5.2): status %d, want 200 or 204", r.path(object), res.StatusCode)
	}
	if res.StatusCode == http.StatusOK {
		r.expectLocated(t, res)
	}
}

func (r *resource) expectValues(t *testing.T, object core.Object, name string, want ...string) {
	t.Helper()

	want = slices.Sorted(slices.Values(want))
	if got := valuesIn(r.read(t, object), name); !slices.Equal(got, want) {
		t.Errorf("scimtest: GET %s (RFC 7644 3.5.2): %s values %v, want %v", r.path(object), name, got, want)
	}
}

// RFC 7643 Section 3.1: meta.location is the URI of the resource and MUST equal "Content-Location".
func (r *resource) expectLocated(t *testing.T, res *http.Response) {
	t.Helper()

	object := Decode[core.Object](t, res)
	location, _ := metaOf(object)["location"].(string)
	where := res.Request.Method + " " + res.Request.URL.Path
	if location != "" && !strings.HasSuffix(location, r.path(object)) {
		t.Errorf("scimtest: %s (RFC 7643 3.1): meta.location %q, want the URI of %s", where, location, r.path(object))
	}
	if got := res.Header.Get("Content-Location"); got != location {
		t.Errorf("scimtest: %s (RFC 7643 3.1): Content-Location %q, want meta.location %q", where, got, location)
	}
}

func (r *resource) send(t *testing.T, query url.Values) *http.Response {
	t.Helper()
	return r.transport.send(t, r.client(), r.kind.Endpoint, query)
}

func (r *resource) list(t *testing.T, query url.Values) protocol.ListResponse[core.Object] {
	t.Helper()

	return r.page(t, query, r.send(t, query))
}

func (r *resource) page(t *testing.T, query url.Values, res *http.Response) protocol.ListResponse[core.Object] {
	t.Helper()

	if res.StatusCode != http.StatusOK {
		t.Fatalf("scimtest: %s (RFC 7644 3.4.2): status %d, want 200", r.transport.describe(r.kind.Endpoint, query), res.StatusCode)
	}
	page := Decode[protocol.ListResponse[core.Object]](t, res)
	if !slices.Contains(page.Schemas, protocol.SchemaListResponse) {
		t.Fatalf("scimtest: %s (RFC 7644 3.4.2): schemas %v, want %s", r.transport.describe(r.kind.Endpoint, query), page.Schemas, protocol.SchemaListResponse)
	}
	return page
}

func (r *resource) read(t *testing.T, object core.Object) core.Object {
	t.Helper()

	res := r.client().Get(t, r.path(object))
	expectStatus(t, res, http.StatusOK, "RFC 7644 3.4.1")
	return Decode[core.Object](t, res)
}

func (r *resource) filtered(t *testing.T, query url.Values) []string {
	t.Helper()

	res := r.send(t, query)
	if res.StatusCode == http.StatusBadRequest {
		RequireError(t, res, http.StatusBadRequest, scimerrors.InvalidFilter)
		r.suite.todo(t, "%s returned 400 invalidFilter", r.transport.describe(r.kind.Endpoint, query))
	}
	return idsOf(r.page(t, query, res).Resources)
}

func (r *resource) expectMatches(t *testing.T, filter string, ours, want []string) {
	t.Helper()

	query := filterBy("(" + filter + ") and (" + idFilter(ours...) + ")")
	query.Set(countParam, strconv.Itoa(len(ours)))
	found := r.filtered(t, query)
	got := slices.DeleteFunc(slices.Clone(ours), func(id string) bool { return !slices.Contains(found, id) })
	if !slices.Equal(got, want) {
		t.Errorf("scimtest: %s (RFC 7644 3.4.2.2)\n        want ids: %v\n        got ids:  %v", r.transport.describe(r.kind.Endpoint, query), want, got)
	}
}

func (r *resource) expectIDs(t *testing.T, query url.Values, want []string, section string) {
	t.Helper()

	if got := idsOf(r.list(t, query).Resources); !slices.Equal(got, want) {
		t.Errorf("scimtest: %s (%s): ids %v, want %v", r.transport.describe(r.kind.Endpoint, query), section, got, want)
	}
}

func (r *resource) expectPage(t *testing.T, query url.Values, most, total int) {
	t.Helper()

	page := r.list(t, query)
	if len(page.Resources) > most || page.TotalResults < total {
		t.Errorf("scimtest: %s (RFC 7644 3.4.2.4): %d resources of %d, want at most %d of at least %d", r.transport.describe(r.kind.Endpoint, query), len(page.Resources), page.TotalResults, most, total)
	}
}

func (r *resource) expectAttributes(t *testing.T, path string, present, absent []string) {
	t.Helper()

	res := r.client().Get(t, path)
	expectStatus(t, res, http.StatusOK, "RFC 7644 3.4.2.5")
	got := Decode[core.Object](t, res)
	for _, name := range present {
		if !got.Has(name) {
			t.Errorf("scimtest: GET %s (RFC 7644 3.4.2.5): %s is missing", path, name)
		}
	}
	for _, name := range absent {
		if got.Has(name) {
			t.Errorf("scimtest: GET %s (RFC 7644 3.4.2.5): %s is present", path, name)
		}
	}
}

func (r *resource) create(t *testing.T, unique string) (*http.Response, core.Object) {
	t.Helper()
	return r.createBody(t, r.body(t, unique))
}

func (r *resource) createBody(t *testing.T, body core.Object) (*http.Response, core.Object) {
	t.Helper()

	res, created := r.post(t, body)
	expectStatus(t, res, http.StatusCreated, "RFC 7644 3.3")
	if idOf(created) == "" {
		t.Fatalf("scimtest: POST %s (RFC 7643 3.1): no id", r.kind.Endpoint)
	}
	return res, created
}

func (r *resource) post(t *testing.T, body core.Object) (*http.Response, core.Object) {
	t.Helper()

	res := r.client().Post(t, r.kind.Endpoint, body)
	if res.StatusCode != http.StatusCreated {
		return res, nil
	}
	created := Decode[core.Object](t, res)
	if idOf(created) != "" {
		path := r.path(created)
		t.Cleanup(func() { r.remove(path) })
	}
	return res, created
}

func (r *resource) seed(t *testing.T, body core.Object) core.Object {
	t.Helper()

	_, created := r.post(t, body)
	if created == nil {
		t.Skipf("n/a: the server rejected %v", body)
	}
	return created
}

func (r *resource) remove(path string) {
	if res, err := r.client().send(context.Background(), http.MethodDelete, path, nil); err == nil {
		_ = res.Body.Close()
	}
}

func (r *resource) body(t *testing.T, unique string) core.Object {
	t.Helper()

	if r.fixture == nil {
		r.suite.todo(t, "no fixture for %s", r.kind.Name)
	}
	object, err := r.fixture(r.schemas.Base(), unique)
	if err != nil {
		t.Fatalf("scimtest: %s fixture: %v", r.kind.Name, err)
	}
	object.Remove("id")
	object.Remove("meta")
	object.Set("schemas", r.schemaURIs(object))
	return object
}

func (r *resource) schemaURIs(object core.Object) []core.SchemaURI {
	uris := []core.SchemaURI{r.kind.Schema}
	for _, extension := range r.kind.SchemaExtensions {
		if isEmptyObject(object.Get(string(extension.Schema))) {
			object.Remove(string(extension.Schema))
			continue
		}
		uris = append(uris, extension.Schema)
	}
	return uris
}

func (r *resource) path(object core.Object) string {
	return r.kind.Endpoint + "/" + idOf(object)
}

func (r *resource) client() *Client {
	return r.suite.client
}

func supports(t *testing.T, feature string, supported bool) {
	t.Helper()

	if !supported {
		t.Skipf("MAY: %s.supported is false", feature)
	}
}

func expectIgnored(t *testing.T, where string, value any, marker string) {
	t.Helper()

	if raw, _ := json.Marshal(value); strings.Contains(string(raw), marker) {
		t.Errorf("scimtest: %s: kept the readOnly value %s from the request", where, marker)
	}
}

func isReadOnly(attribute *core.Attribute) bool {
	return attribute.Mutability == core.MutabilityReadOnly
}

func readOnlyValue(attribute *core.Attribute, marker string) any {
	value := any(marker)
	if attribute.Type == core.TypeComplex {
		value = map[string]any{valueSub: marker}
	}
	if attribute.MultiValued {
		value = []any{value}
	}
	return value
}

func patchable(attribute *core.Attribute) bool {
	return isFilterable(attribute) && !isUnique(attribute)
}

func required(object core.Object) func(*core.Attribute) bool {
	return func(a *core.Attribute) bool { return a.Required && a.Name != "id" && object.Has(a.Name) }
}

func find(schema *core.Schema, match func(*core.Attribute) bool) *core.Attribute {
	if i := slices.IndexFunc(schema.Attributes, match); i >= 0 {
		return schema.Attributes[i]
	}
	return nil
}

func ops(operations ...patch.Operation) protocol.PatchRequest {
	return protocol.PatchRequest{Schemas: []core.SchemaURI{protocol.SchemaPatchOp}, Operations: operations}
}

func op(kind patch.Op, path string, value any) patch.Operation {
	operation := patch.Operation{Op: kind, Path: path}
	if value != nil {
		operation.Value, _ = json.Marshal(value)
	}
	return operation
}

func byID(ids ...string) url.Values {
	return filterBy(idFilter(ids...))
}

func idFilter(ids ...string) string {
	terms := make([]string, 0, len(ids))
	for _, id := range ids {
		terms = append(terms, "id eq "+quoted(id))
	}
	return strings.Join(terms, " or ")
}

func quoted(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func metaOf(object core.Object) map[string]any {
	meta, _ := object.Get("meta").(map[string]any)
	return meta
}

func filterBy(expression string) url.Values {
	return url.Values{"filter": {expression}}
}

func idOf(object core.Object) string {
	id, _ := object.Get("id").(string)
	return id
}

func idsOf(objects []core.Object) []string {
	ids := make([]string, 0, len(objects))
	for _, object := range objects {
		ids = append(ids, idOf(object))
	}
	return ids
}

func isEmptyObject(value any) bool {
	object, ok := value.(map[string]any)
	return value == nil || ok && len(object) == 0
}
