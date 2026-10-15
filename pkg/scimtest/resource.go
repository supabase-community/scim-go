package scimtest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type resource struct {
	suite   *Suite
	config  *core.ServiceProviderConfig
	kind    core.ResourceType
	schema  *core.Schema
	fixture fixture
}

func (r *resource) checkCreate(t *testing.T) {
	t.Run("returns 201 with a Location", r.checkLocation)
	t.Run("ignores a readOnly id", r.checkReadOnly)
	t.Run("rejects a duplicate unique value", r.checkUniqueness)
	t.Run("rejects a missing required attribute", r.checkRequired)
}

func (r *resource) checkLocation(t *testing.T) {
	res, created := r.create(t, newUnique())
	location, _ := metaOf(created)["location"].(string)
	if got := res.Header.Get("Location"); got == "" || got != location {
		t.Errorf("scimtest: POST %s (RFC 7644 3.3): Location %q, want meta.location %q", r.kind.Endpoint, got, location)
	}
}

func (r *resource) checkReadOnly(t *testing.T) {
	body := r.body(t, newUnique())
	body.Set("id", "scimtest-readonly")
	if _, created := r.createBody(t, body); idOf(created) == "scimtest-readonly" {
		t.Errorf("scimtest: POST %s (RFC 7644 3.3): kept the readOnly id from the request", r.kind.Endpoint)
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
	list[core.Object](t, r.client(), r.kind.Endpoint)
}

func (r *resource) checkFilter(t *testing.T) {
	t.Run("finds a resource by id", r.checkFilterID)
	t.Run("rejects an invalid filter", r.checkFilterInvalid)
	t.Run("rejects gt on a boolean", r.checkFilterBoolean)
	t.Run("operators", r.checkFilterOperators)
}

func (r *resource) checkFilterBoolean(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	attribute := r.attribute(t, "singular boolean attribute", func(a *core.Attribute) bool { return a.Type == core.TypeBoolean && !a.MultiValued })
	path := r.kind.Endpoint + "?" + filterBy(attribute.Name+" gt true").Encode()

	RequireError(t, r.client().Get(t, path), http.StatusBadRequest, scimerrors.InvalidFilter)
}

func (r *resource) checkFilterOperators(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	newFilterTable(t, r).run(t)
}

func (r *resource) checkFilterID(t *testing.T) {
	if r.suite.unsupported(t, "filter", r.config.Filter.Supported) {
		res := r.client().Get(t, r.kind.Endpoint+"?"+byID("scimtest-none").Encode())
		expectStatus(t, res, http.StatusNotImplemented, "RFC 7644 3.12")
		return
	}
	_, created := r.create(t, newUnique())
	query := byID(idOf(created))

	if got := r.filtered(t, query); !slices.Equal(got, []string{idOf(created)}) {
		t.Errorf("scimtest: GET %s?%s (RFC 7644 3.4.2.2): ids %v, want [%s]", r.kind.Endpoint, query.Encode(), got, idOf(created))
	}
}

func (r *resource) checkFilterInvalid(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	r.suite.should(t, "RFC 7644 Table 9 400 invalidFilter for an invalid filter")
	path := r.kind.Endpoint + "?filter=id+eq"

	RequireError(t, r.client().Get(t, path), http.StatusBadRequest, scimerrors.InvalidFilter)
}

func (r *resource) checkSort(t *testing.T) {
	t.Run("accepts sortBy", r.checkSortBy)
	t.Run("sorts ascending by default and descending on request", r.checkSortOrder)
}

func (r *resource) checkSortBy(t *testing.T) {
	path := r.kind.Endpoint + "?sortBy=id"
	if r.suite.unsupported(t, "sort", r.config.Sort.Supported) {
		expectStatus(t, r.client().Get(t, path), http.StatusNotImplemented, "RFC 7644 3.12")
		return
	}
	list[core.Object](t, r.client(), path)
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
	query.Set("sortOrder", "descending")
	r.expectIDs(t, query, []string{ids[1], ids[0]}, "RFC 7644 3.4.2.3")
}

func (r *resource) checkPagination(t *testing.T) {
	r.create(t, newUnique())
	r.create(t, newUnique())

	t.Run("returns only totalResults for count 0", func(t *testing.T) { r.expectPage(t, "count=0", 0, 1) })
	t.Run("treats a negative count as 0", func(t *testing.T) { r.expectPage(t, "count=-1", 0, 1) })
	t.Run("returns no more than count", func(t *testing.T) { r.expectPage(t, "count=1", 1, 2) })
	t.Run("treats a startIndex below 1 as 1", func(t *testing.T) {
		page := list[core.Object](t, r.client(), r.kind.Endpoint+"?startIndex=0&count=1")
		if page.StartIndex != 1 {
			t.Errorf("scimtest: GET %s?startIndex=0 (RFC 7644 3.4.2.4): startIndex %d, want 1", r.kind.Endpoint, page.StartIndex)
		}
	})
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

func (r *resource) checkSearch(t *testing.T) {
	r.create(t, newUnique())
	body := map[string]any{"schemas": []core.SchemaURI{protocol.SchemaSearchRequest}, "startIndex": 1, "count": 1}

	res := r.client().Post(t, r.kind.Endpoint+"/.search", body)
	if res.StatusCode != http.StatusOK {
		r.suite.todo(t, "POST %s/.search returned %d", r.kind.Endpoint, res.StatusCode)
	}
	if page := Decode[protocol.ListResponse[core.Object]](t, res); !slices.Contains(page.Schemas, protocol.SchemaListResponse) || len(page.Resources) > 1 {
		t.Errorf("scimtest: POST %s/.search (RFC 7644 3.4.3): schemas %v with %d resources, want a ListResponse of at most 1", r.kind.Endpoint, page.Schemas, len(page.Resources))
	}
}

func (r *resource) checkReplace(t *testing.T) {
	t.Run("replaces the resource", r.checkPut)
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

func (r *resource) checkPatch(t *testing.T) {
	t.Run("replaces an attribute", r.checkPatchReplace)
	t.Run("rejects removing a required attribute", r.checkPatchRequired)
	t.Run("adds attributes without a path", r.checkPatchAdd)
	t.Run("unassigns a removed attribute", r.checkPatchRemove)
	t.Run("rejects a remove without a path", r.checkPatchNoPath)
	t.Run("rejects a value filter without a match", r.checkPatchNoMatch)
	t.Run("is atomic", r.checkPatchAtomic)
	t.Run("rejects a body without the PatchOp schema", r.checkPatchSchema)
}

func (r *resource) checkPatchReplace(t *testing.T) {
	if r.suite.unsupported(t, "patch", r.config.Patch.Supported) {
		_, created := r.create(t, newUnique())
		res := r.client().Patch(t, r.path(created), ops(op(patch.OpReplace, "externalId", "scimtest")))
		expectStatus(t, res, http.StatusNotImplemented, "RFC 7644 3.12")
		return
	}
	attribute := find(r.schema, patchable)
	if attribute == nil {
		r.suite.todo(t, "%s has no readWrite singular string attribute to PATCH", r.kind.Name)
	}
	_, created := r.create(t, newUnique())
	want := "scimtest " + newUnique()
	r.expectPatched(t, r.client().Patch(t, r.path(created), ops(op(patch.OpReplace, attribute.Name, want))), created)
	r.expectValue(t, created, attribute.Name, want)
}

func (r *resource) checkPatchRequired(t *testing.T) {
	supports(t, "patch", r.config.Patch.Supported)
	body := r.body(t, newUnique())
	attribute := r.attribute(t, "required attribute in its fixture", required(body))
	_, created := r.createBody(t, body)

	RequireError(t, r.client().Patch(t, r.path(created), ops(op(patch.OpRemove, attribute.Name, nil))), http.StatusBadRequest, scimerrors.Mutability)
}

func (r *resource) checkPatchAdd(t *testing.T) {
	created, attribute := r.patchTarget(t)
	want := "scimtest " + newUnique()

	res := r.client().Patch(t, r.path(created), ops(op(patch.OpAdd, "", map[string]string{attribute.Name: want})))
	r.expectPatched(t, res, created)
	r.expectValue(t, created, attribute.Name, want)
}

func (r *resource) checkPatchRemove(t *testing.T) {
	attribute := r.patchAttribute(t)
	if attribute.Required {
		t.Skipf("n/a: %s is required", attribute.Name)
	}
	body := r.body(t, newUnique())
	body.Set(attribute.Name, "scimtest")
	_, created := r.createBody(t, body)
	if created.Get(attribute.Name) != "scimtest" {
		t.Skipf("n/a: the server did not store %s", attribute.Name)
	}

	r.expectPatched(t, r.client().Patch(t, r.path(created), ops(op(patch.OpRemove, attribute.Name, nil))), created)
	r.expectValue(t, created, attribute.Name, nil)
}

func (r *resource) checkPatchNoPath(t *testing.T) {
	created, _ := r.patchTarget(t)

	RequireError(t, r.client().Patch(t, r.path(created), ops(op(patch.OpRemove, "", nil))), http.StatusBadRequest, scimerrors.NoTarget)
}

func (r *resource) checkPatchNoMatch(t *testing.T) {
	created, _ := r.patchTarget(t)
	attribute := r.attribute(t, "readWrite multi-valued complex attribute with a value", func(a *core.Attribute) bool {
		return a.MultiValued && a.Type == core.TypeComplex && a.Mutability == core.MutabilityReadWrite && a.SubAttributes.Lookup("value") != nil
	})
	path := attribute.Name + `[value eq "scimtest-none"]`

	RequireError(t, r.client().Patch(t, r.path(created), ops(op(patch.OpReplace, path, map[string]string{"value": "scimtest"}))), http.StatusBadRequest, scimerrors.NoTarget)
}

func (r *resource) checkPatchAtomic(t *testing.T) {
	created, attribute := r.patchTarget(t)
	remove := r.attribute(t, "required attribute", required(created))

	res := r.client().Patch(t, r.path(created), ops(op(patch.OpReplace, attribute.Name, "scimtest "+newUnique()), op(patch.OpRemove, remove.Name, nil)))
	if res.StatusCode < http.StatusBadRequest {
		t.Fatalf("scimtest: PATCH %s (RFC 7644 3.5.2): status %d, want a failure for removing %s", r.path(created), res.StatusCode, remove.Name)
	}
	r.expectValue(t, created, attribute.Name, created.Get(attribute.Name))
}

func (r *resource) checkPatchSchema(t *testing.T) {
	r.suite.should(t, "RFC 7644 Table 8 400 for a PATCH body without the PatchOp schema")
	created, attribute := r.patchTarget(t)
	body := ops(op(patch.OpReplace, attribute.Name, "scimtest"))
	body.Schemas = nil

	expectStatus(t, r.client().Patch(t, r.path(created), body), http.StatusBadRequest, "RFC 7644 3.5.2")
}

func (r *resource) checkDelete(t *testing.T) {
	_, created := r.create(t, newUnique())

	expectStatus(t, r.client().Delete(t, r.path(created)), http.StatusNoContent, "RFC 7644 3.6")
	expectStatus(t, r.client().Get(t, r.path(created)), http.StatusNotFound, "RFC 7644 3.6")
}

func (r *resource) checkVersioning(t *testing.T) {
	supports(t, "etag", r.config.ETag.Supported)
	unique := newUnique()
	res, created := r.create(t, unique)
	etag := res.Header.Get("ETag")
	if etag == "" {
		t.Errorf("scimtest: POST %s (RFC 7644 3.14): no ETag header", r.kind.Endpoint)
	}
	if version := metaOf(created)["version"]; r.suite.strict && version != etag {
		t.Errorf("SHOULD: POST %s (RFC 7644 3.14): meta.version %v, want ETag %q", r.kind.Endpoint, version, etag)
	}

	stale := r.client().With(WithHeader("If-Match", `W/"scimtest-stale"`)).Put(t, r.path(created), r.body(t, unique))
	expectStatus(t, stale, http.StatusPreconditionFailed, "RFC 7232 3.1")
}

func (r *resource) patchTarget(t *testing.T) (core.Object, *core.Attribute) {
	t.Helper()

	attribute := r.patchAttribute(t)
	_, created := r.create(t, newUnique())
	return created, attribute
}

func (r *resource) patchAttribute(t *testing.T) *core.Attribute {
	t.Helper()

	supports(t, "patch", r.config.Patch.Supported)
	return r.attribute(t, "readWrite singular string attribute", patchable)
}

func (r *resource) attribute(t *testing.T, what string, match func(*core.Attribute) bool) *core.Attribute {
	t.Helper()

	attribute := find(r.schema, match)
	if attribute == nil {
		t.Skipf("n/a: %s has no %s", r.kind.Name, what)
	}
	return attribute
}

func (r *resource) read(t *testing.T, object core.Object) core.Object {
	t.Helper()

	res := r.client().Get(t, r.path(object))
	expectStatus(t, res, http.StatusOK, "RFC 7644 3.4.1")
	return Decode[core.Object](t, res)
}

func (r *resource) expectPatched(t *testing.T, res *http.Response, object core.Object) {
	t.Helper()

	if res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusOK {
		t.Fatalf("scimtest: PATCH %s (RFC 7644 3.5.2): status %d, want 200 or 204", r.path(object), res.StatusCode)
	}
}

func (r *resource) expectValue(t *testing.T, object core.Object, name string, want any) {
	t.Helper()

	if got := r.read(t, object).Get(name); got != want {
		t.Errorf("scimtest: GET %s (RFC 7644 3.5.2): %s is %v, want %v", r.path(object), name, got, want)
	}
}

func (r *resource) filtered(t *testing.T, query url.Values) []string {
	t.Helper()

	path := r.kind.Endpoint + "?" + query.Encode()
	res := r.client().Get(t, path)
	if res.StatusCode == http.StatusBadRequest {
		RequireError(t, res, http.StatusBadRequest, scimerrors.InvalidFilter)
		r.suite.todo(t, "GET %s returned 400 invalidFilter", path)
	}
	expectStatus(t, res, http.StatusOK, "RFC 7644 3.4.2.2")
	return idsOf(Decode[protocol.ListResponse[core.Object]](t, res).Resources)
}

func (r *resource) expectIDs(t *testing.T, query url.Values, want []string, section string) {
	t.Helper()

	path := r.kind.Endpoint + "?" + query.Encode()
	if got := idsOf(list[core.Object](t, r.client(), path).Resources); !slices.Equal(got, want) {
		t.Errorf("scimtest: GET %s (%s): ids %v, want %v", path, section, got, want)
	}
}

func (r *resource) expectPage(t *testing.T, query string, most, total int) {
	t.Helper()

	page := list[core.Object](t, r.client(), r.kind.Endpoint+"?"+query)
	if len(page.Resources) > most || page.TotalResults < total {
		t.Errorf("scimtest: GET %s?%s (RFC 7644 3.4.2.4): %d resources of %d, want at most %d of at least %d", r.kind.Endpoint, query, len(page.Resources), page.TotalResults, most, total)
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
	object, err := r.fixture(r.schema, unique)
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
