package scimtest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

const (
	typeSub     = "type"
	valueSub    = "value"
	primarySub  = "primary"
	placeholder = "scimtest"
	schemasAttr = "schemas"
)

type resource struct {
	suite     *Suite
	config    *core.ServiceProviderConfig
	kind      core.ResourceType
	schemas   core.Schemas
	fixture   fixture
	full      fixture
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

func (r *resource) attribute(t *testing.T, what string, match func(*core.Attribute) bool) *core.Attribute {
	t.Helper()

	attribute := firstOf(r.schemas.Base().Attributes, match)
	if attribute == nil {
		t.Skipf("n/a: %s has no %s", r.kind.Name, what)
	}
	return attribute
}

func (r *resource) extended(t *testing.T, match func(*core.Attribute) bool) (*core.Schema, *core.Attribute) {
	t.Helper()

	extension, attribute := extensionAttribute(r.schemas, match)
	if extension == nil {
		t.Skipf("n/a: %s has no extension with a readWrite singular string attribute", r.kind.Name)
	}
	return extension, attribute
}

func (r *resource) complexTarget(t *testing.T) (*core.Attribute, core.Object, string, string) {
	t.Helper()

	attribute := r.attribute(t, "readWrite singular complex attribute with two string sub-attributes", func(a *core.Attribute) bool {
		return a.Type == core.TypeComplex && !a.MultiValued && a.Mutability == core.MutabilityReadWrite && len(filterableSubs(a)) > 1
	})
	subs := filterableSubs(attribute)
	body := r.body(t, newUnique())
	body.Set(attribute.Name, filled(subs[:2]))
	_, created := r.createBody(t, body)
	return attribute, created, subs[0].Name, subs[1].Name
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

func (r *resource) describe(query url.Values) string {
	return r.transport.describe(r.kind.Endpoint, query)
}

func (r *resource) list(t *testing.T, query url.Values) protocol.ListResponse[core.Object] {
	t.Helper()

	return r.page(t, query, r.send(t, query))
}

func (r *resource) page(t *testing.T, query url.Values, res *http.Response) protocol.ListResponse[core.Object] {
	t.Helper()
	return listed[core.Object](t, r.describe(query), res)
}

func (r *resource) read(t *testing.T, object core.Object) core.Object {
	t.Helper()

	return get[core.Object](t, r.client(), r.path(object), "RFC 7644 3.4.1")
}

func (r *resource) filtered(t *testing.T, query url.Values) []string {
	t.Helper()

	res := r.send(t, query)
	if res.StatusCode == http.StatusBadRequest {
		RequireError(t, res, http.StatusBadRequest, scimerrors.InvalidFilter)
		r.suite.todo(t, "%s returned 400 invalidFilter", r.describe(query))
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
		t.Errorf("scimtest: %s (RFC 7644 3.4.2.2)\n        want ids: %v\n        got ids:  %v", r.describe(query), want, got)
	}
}

func (r *resource) expectPage(t *testing.T, query url.Values, most, total int) {
	t.Helper()

	page := r.list(t, query)
	if len(page.Resources) > most || page.TotalResults < total {
		t.Errorf("scimtest: %s (RFC 7644 3.4.2.4): %d resources of %d, want at most %d of at least %d", r.describe(query), len(page.Resources), page.TotalResults, most, total)
	}
}

func (r *resource) create(t *testing.T, unique string) (*http.Response, core.Object) {
	t.Helper()
	return r.createBody(t, r.body(t, unique))
}

func (r *resource) createIDs(t *testing.T, n int) []string {
	t.Helper()

	ids := make([]string, 0, n)
	for range n {
		_, created := r.create(t, newUnique())
		ids = append(ids, idOf(created))
	}
	return ids
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
	return r.build(t, r.fixture, unique)
}

func (r *resource) fullBody(t *testing.T) core.Object {
	t.Helper()

	if r.full == nil {
		t.Skipf("n/a: no full fixture for %s", r.kind.Name)
	}
	body := r.build(t, r.full, newUnique())
	for _, h := range holders(r.schemas, body) {
		for _, attribute := range h.schema.Attributes {
			if isReference(attribute) {
				h.object.Remove(attribute.Name)
			}
			compactBinary(attribute, h.object.Get(attribute.Name))
		}
	}
	body.Set(schemasAttr, r.schemaURIs(body))
	return body
}

func (r *resource) build(t *testing.T, fx fixture, unique string) core.Object {
	t.Helper()

	object, err := fx(r.schemas.Base(), unique)
	if err != nil {
		t.Fatalf("scimtest: %s fixture: %v", r.kind.Name, err)
	}
	object.Remove("id")
	object.Remove("meta")
	object.Set(schemasAttr, r.schemaURIs(object))
	return object
}

func (r *resource) withExtension(body core.Object, extension *core.Schema, name string, value any) {
	body.Set(string(extension.ID), map[string]any{name: value})
	body.Set(schemasAttr, r.schemaURIs(body))
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

func patchable(attribute *core.Attribute) bool {
	return isFilterable(attribute) && !isUnique(attribute)
}

// RFC 7643 Section 2.3.6: binary values MUST be base64, so drop the spaces of the Section 8.3 example.
func compactBinary(attribute *core.Attribute, value any) {
	for _, element := range elementsOf(value) {
		for _, sub := range attribute.SubAttributes {
			if text, ok := core.Object(element).Get(sub.Name).(string); ok && sub.Type == core.TypeBinary {
				core.Object(element).Set(sub.Name, strings.Join(strings.Fields(text), ""))
			}
		}
	}
}

func (r *resource) mistype(t *testing.T, body core.Object) {
	t.Helper()

	writable := func(kind core.AttributeType) func(*core.Attribute) bool {
		return func(a *core.Attribute) bool {
			return a.Type == kind && !a.MultiValued && a.Mutability == core.MutabilityReadWrite
		}
	}
	if attribute := firstOf(r.schemas.Base().Attributes, writable(core.TypeBoolean)); attribute != nil {
		body.Set(attribute.Name, placeholder)
		return
	}
	body.Set(r.attribute(t, "readWrite singular boolean or string attribute", writable(core.TypeString)).Name, 5)
}

// RFC 7644 Section 3.12, Table 9: a value incompatible with the attribute type is invalidValue, or invalidSyntax for the body.
func expectTypeError(t *testing.T, res *http.Response) {
	t.Helper()

	if body := RequireError(t, res, http.StatusBadRequest, ""); body.ScimType != scimerrors.InvalidValue && body.ScimType != scimerrors.InvalidSyntax {
		t.Errorf("scimtest: %s %s (RFC 7644 3.12): scimType %q, want invalidValue or invalidSyntax", res.Request.Method, res.Request.URL.Path, body.ScimType)
	}
}
