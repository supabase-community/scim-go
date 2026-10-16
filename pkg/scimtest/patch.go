package scimtest

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type patching struct {
	*resource
}

func (r patching) run(t *testing.T) {
	t.Helper()

	t.Run("replaces an attribute", r.checkReplaceByPath)
	t.Run("rejects removing a required attribute", r.checkRequired)
	t.Run("adds attributes without a path", r.checkAddWithoutPath)
	t.Run("unassigns a removed attribute", r.checkRemove)
	t.Run("rejects a remove without a path", r.checkNoPath)
	t.Run("rejects a value filter without a match", r.checkNoMatch)
	t.Run("is atomic", r.checkAtomic)
	t.Run("rejects a body without the PatchOp schema", r.checkSchema)
	t.Run("adds an attribute by path", r.checkAddByPath)
	t.Run("adds an unassigned attribute on replace", r.checkReplaceUnassigned)
	t.Run("keeps meta.lastModified when adding the current value", r.checkUnchanged)
	t.Run("adds and removes multi-valued values", r.checkValues)
	t.Run("keeps a single primary value", r.checkPrimary)
	t.Run("adds the extension schema with an extension attribute", r.checkExtension)
	t.Run("rejects an unknown op", r.checkUnknownOp)
	t.Run("rejects an unknown path", r.checkUnknownPath)
	t.Run("rejects a readOnly id", r.checkReadOnly)
	t.Run("rejects a body without operations", r.checkEmpty)
	t.Run("returns 200 with the requested attributes", r.checkProjected)
}

func (r patching) checkReplaceByPath(t *testing.T) {
	if r.suite.unsupported(t, "patch", r.config.Patch.Supported) {
		_, created := r.create(t, newUnique())
		res := r.client().Patch(t, r.path(created), ops(op(patch.OpReplace, "externalId", placeholder)))
		expectStatus(t, res, http.StatusNotImplemented, "RFC 7644 3.12")
		return
	}
	attribute := find(r.schemas.Base(), patchable)
	if attribute == nil {
		r.suite.todo(t, "%s has no readWrite singular string attribute to PATCH", r.kind.Name)
	}
	_, created := r.create(t, newUnique())
	want := "scimtest " + newUnique()
	r.expectPatched(t, r.client().Patch(t, r.path(created), ops(op(patch.OpReplace, attribute.Name, want))), created)
	r.expectValue(t, created, attribute.Name, want)
}

func (r patching) checkRequired(t *testing.T) {
	supports(t, "patch", r.config.Patch.Supported)
	body := r.body(t, newUnique())
	attribute := r.attribute(t, "required attribute in its fixture", required(body))
	_, created := r.createBody(t, body)

	RequireError(t, r.client().Patch(t, r.path(created), ops(op(patch.OpRemove, attribute.Name, nil))), http.StatusBadRequest, scimerrors.Mutability)
}

func (r patching) checkAddWithoutPath(t *testing.T) {
	created, attribute := r.patchTarget(t)
	want := "scimtest " + newUnique()

	res := r.client().Patch(t, r.path(created), ops(op(patch.OpAdd, "", map[string]string{attribute.Name: want})))
	r.expectPatched(t, res, created)
	r.expectValue(t, created, attribute.Name, want)
}

func (r patching) checkRemove(t *testing.T) {
	attribute := r.patchAttribute(t)
	if attribute.Required {
		t.Skipf("n/a: %s is required", attribute.Name)
	}
	body := r.body(t, newUnique())
	body.Set(attribute.Name, placeholder)
	_, created := r.createBody(t, body)
	if created.Get(attribute.Name) != placeholder {
		t.Skipf("n/a: the server did not store %s", attribute.Name)
	}

	r.expectPatched(t, r.client().Patch(t, r.path(created), ops(op(patch.OpRemove, attribute.Name, nil))), created)
	r.expectValue(t, created, attribute.Name, nil)
}

func (r patching) checkNoPath(t *testing.T) {
	created, _ := r.patchTarget(t)

	RequireError(t, r.client().Patch(t, r.path(created), ops(op(patch.OpRemove, "", nil))), http.StatusBadRequest, scimerrors.NoTarget)
}

func (r patching) checkNoMatch(t *testing.T) {
	created, _ := r.patchTarget(t)
	attribute := r.attribute(t, "readWrite multi-valued complex attribute with a value", func(a *core.Attribute) bool {
		return a.MultiValued && a.Type == core.TypeComplex && a.Mutability == core.MutabilityReadWrite && a.SubAttributes.Lookup(valueSub) != nil
	})
	path := attribute.Name + `[value eq "scimtest-none"]`

	RequireError(t, r.client().Patch(t, r.path(created), ops(op(patch.OpReplace, path, map[string]string{valueSub: placeholder}))), http.StatusBadRequest, scimerrors.NoTarget)
}

func (r patching) checkAtomic(t *testing.T) {
	created, attribute := r.patchTarget(t)
	remove := r.attribute(t, "required attribute", required(created))

	res := r.client().Patch(t, r.path(created), ops(op(patch.OpReplace, attribute.Name, "scimtest "+newUnique()), op(patch.OpRemove, remove.Name, nil)))
	if res.StatusCode < http.StatusBadRequest {
		t.Fatalf("scimtest: PATCH %s (RFC 7644 3.5.2): status %d, want a failure for removing %s", r.path(created), res.StatusCode, remove.Name)
	}
	r.expectValue(t, created, attribute.Name, created.Get(attribute.Name))
}

func (r patching) checkSchema(t *testing.T) {
	r.suite.should(t, "RFC 7644 Table 8 400 for a PATCH body without the PatchOp schema")
	created, attribute := r.patchTarget(t)
	body := ops(op(patch.OpReplace, attribute.Name, placeholder))
	body.Schemas = nil

	expectStatus(t, r.client().Patch(t, r.path(created), body), http.StatusBadRequest, "RFC 7644 3.5.2")
}

func (r patching) checkAddByPath(t *testing.T) {
	created, attribute := r.patchTarget(t)
	want := placeholder + " " + newUnique()

	r.expectPatched(t, r.client().Patch(t, r.path(created), ops(op(patch.OpAdd, attribute.Name, want))), created)
	r.expectValue(t, created, attribute.Name, want)
}

// RFC 7644 Section 3.5.2.3: replacing an attribute that does not exist SHALL be treated as an "add".
func (r patching) checkReplaceUnassigned(t *testing.T) {
	attribute := r.patchAttribute(t)
	if attribute.Required {
		t.Skipf("n/a: %s is required", attribute.Name)
	}
	body := r.body(t, newUnique())
	body.Remove(attribute.Name)
	_, created := r.createBody(t, body)
	want := placeholder + " " + newUnique()

	r.expectPatched(t, r.client().Patch(t, r.path(created), ops(op(patch.OpReplace, attribute.Name, want))), created)
	r.expectValue(t, created, attribute.Name, want)
}

// RFC 7644 Section 3.5.2.1: adding the current value SHALL NOT change the modify timestamp.
func (r patching) checkUnchanged(t *testing.T) {
	attribute := r.patchAttribute(t)
	body := r.body(t, newUnique())
	current := placeholder + " " + newUnique()
	body.Set(attribute.Name, current)
	_, created := r.createBody(t, body)

	r.expectPatched(t, r.client().Patch(t, r.path(created), ops(op(patch.OpAdd, attribute.Name, current))), created)
	if got, want := metaOf(r.read(t, created))["lastModified"], metaOf(created)["lastModified"]; got != want {
		t.Errorf("scimtest: PATCH %s (RFC 7644 3.5.2.1): meta.lastModified %v, want %v", r.path(created), got, want)
	}
}

// RFC 7644 Section 3.5.2.2: a multi-valued attribute without values SHALL be considered unassigned.
func (r patching) checkValues(t *testing.T) {
	attribute := r.valued(t)
	unique := newUnique()
	first, second := "first+"+unique+"@example.com", "second+"+unique+"@example.com"
	body := r.body(t, newUnique())
	body.Set(attribute.Name, []any{map[string]any{valueSub: first}})
	_, created := r.createBody(t, body)

	r.expectPatched(t, r.client().Patch(t, r.path(created), ops(op(patch.OpAdd, attribute.Name, []any{map[string]any{valueSub: second}}))), created)
	r.expectValues(t, created, attribute.Name, first, second)
	r.expectPatched(t, r.client().Patch(t, r.path(created), ops(op(patch.OpRemove, attribute.Name+"[value eq "+quoted(second)+"]", nil))), created)
	r.expectValues(t, created, attribute.Name, first)
	r.expectPatched(t, r.client().Patch(t, r.path(created), ops(op(patch.OpRemove, attribute.Name+"[value eq "+quoted(first)+"]", nil))), created)
	if read := r.read(t, created); read.Has(attribute.Name) {
		t.Errorf("scimtest: PATCH %s (RFC 7644 3.5.2.2): %s is %v, want unassigned", r.path(created), attribute.Name, read.Get(attribute.Name))
	}
}

// RFC 7644 Section 3.5.2: setting "primary" to true SHALL set "primary" to false for the other values.
func (r patching) checkPrimary(t *testing.T) {
	attribute := r.valued(t)
	if attribute.SubAttributes.Lookup("primary") == nil {
		t.Skipf("n/a: %s has no primary", attribute.Name)
	}
	unique := newUnique()
	first, second := "first+"+unique+"@example.com", "second+"+unique+"@example.com"
	body := r.body(t, newUnique())
	body.Set(attribute.Name, []any{map[string]any{valueSub: first, "primary": true}})
	_, created := r.createBody(t, body)

	r.expectPatched(t, r.client().Patch(t, r.path(created), ops(op(patch.OpAdd, attribute.Name, []any{map[string]any{valueSub: second, "primary": true}}))), created)
	primaries := []any{}
	for _, element := range elementsOf(r.read(t, created).Get(attribute.Name)) {
		if element["primary"] == true {
			primaries = append(primaries, element[valueSub])
		}
	}
	if len(primaries) != 1 || primaries[0] != second {
		t.Errorf("scimtest: PATCH %s (RFC 7644 3.5.2): primary values %v, want [%s]", r.path(created), primaries, second)
	}
}

// RFC 7644 Section 3.5.2: adding an extension attribute adds the extension URI to "schemas".
func (r patching) checkExtension(t *testing.T) {
	supports(t, "patch", r.config.Patch.Supported)
	extension, attribute := r.extended(t)
	_, created := r.create(t, newUnique())
	path := string(extension.ID) + ":" + attribute.Name
	want := placeholder + " " + newUnique()

	r.expectPatched(t, r.client().Patch(t, r.path(created), ops(op(patch.OpReplace, path, want))), created)
	read := r.read(t, created)
	if raw, _ := json.Marshal(read.Get("schemas")); !strings.Contains(string(raw), string(extension.ID)) {
		t.Errorf("scimtest: PATCH %s (RFC 7644 3.5.2): schemas %s, want %s", r.path(created), raw, extension.ID)
	}
	if got := core.Object(objectOf(read.Get(string(extension.ID)))).Get(attribute.Name); got != want {
		t.Errorf("scimtest: PATCH %s (RFC 7644 3.5.2): %s is %v, want %v", r.path(created), path, got, want)
	}
}

func (r patching) checkUnknownOp(t *testing.T) {
	created, attribute := r.patchTarget(t)
	body := ops(op(patch.OpReplace, attribute.Name, placeholder))
	body.Operations[0].Op = "scimtest"

	RequireError(t, r.client().Patch(t, r.path(created), body), http.StatusBadRequest, scimerrors.InvalidSyntax)
}

func (r patching) checkUnknownPath(t *testing.T) {
	r.suite.should(t, "RFC 7644 Table 9 400 invalidPath for a path to an undefined attribute")
	created, _ := r.patchTarget(t)

	RequireError(t, r.client().Patch(t, r.path(created), ops(op(patch.OpReplace, "scimtest"+newUnique(), placeholder))), http.StatusBadRequest, scimerrors.InvalidPath)
}

func (r patching) checkReadOnly(t *testing.T) {
	created, _ := r.patchTarget(t)

	RequireError(t, r.client().Patch(t, r.path(created), ops(op(patch.OpReplace, "id", "scimtest-"+newUnique()))), http.StatusBadRequest, scimerrors.Mutability)
}

func (r patching) checkEmpty(t *testing.T) {
	r.suite.should(t, "RFC 7644 3.5.2 400 for a PATCH body without operations")
	created, _ := r.patchTarget(t)

	expectStatus(t, r.client().Patch(t, r.path(created), ops()), http.StatusBadRequest, "RFC 7644 3.5.2")
}

// RFC 7644 Section 3.5.2: the server MUST return 200 OK if the "attributes" parameter is specified.
func (r patching) checkProjected(t *testing.T) {
	created, attribute := r.patchTarget(t)
	other := find(r.schemas.Base(), func(a *core.Attribute) bool {
		return a.Returned == core.ReturnedDefault && a.Name != attribute.Name && created.Has(a.Name)
	})

	res := r.client().Patch(t, r.path(created)+"?attributes="+attribute.Name, ops(op(patch.OpReplace, attribute.Name, placeholder+" "+newUnique())))
	expectStatus(t, res, http.StatusOK, "RFC 7644 3.5.2")
	got := Decode[core.Object](t, res)
	if !got.Has("id") || !got.Has(attribute.Name) {
		t.Errorf("scimtest: PATCH %s?attributes=%s (RFC 7644 3.9): id or %s is missing", r.path(created), attribute.Name, attribute.Name)
	}
	if other != nil && got.Has(other.Name) {
		t.Errorf("scimtest: PATCH %s?attributes=%s (RFC 7644 3.9): returned %s", r.path(created), attribute.Name, other.Name)
	}
}

func (r patching) valued(t *testing.T) *core.Attribute {
	t.Helper()

	supports(t, "patch", r.config.Patch.Supported)
	return r.attribute(t, "readWrite multi-valued complex attribute with a value", func(a *core.Attribute) bool {
		return a.MultiValued && a.Type == core.TypeComplex && a.Mutability == core.MutabilityReadWrite && a.SubAttributes.Lookup(valueSub) != nil && a.SubAttributes.Lookup(valueSub).Mutability == core.MutabilityReadWrite
	})
}

func (r patching) extended(t *testing.T) (*core.Schema, *core.Attribute) {
	t.Helper()

	for _, extension := range r.schemas.Extensions() {
		if attribute := find(extension, patchable); attribute != nil {
			return extension, attribute
		}
	}
	t.Skipf("n/a: %s has no extension with a readWrite singular string attribute", r.kind.Name)
	return nil, nil
}

func (r patching) patchTarget(t *testing.T) (core.Object, *core.Attribute) {
	t.Helper()

	attribute := r.patchAttribute(t)
	_, created := r.create(t, newUnique())
	return created, attribute
}

func (r patching) patchAttribute(t *testing.T) *core.Attribute {
	t.Helper()

	supports(t, "patch", r.config.Patch.Supported)
	return r.attribute(t, "readWrite singular string attribute", patchable)
}

func (r patching) expectValue(t *testing.T, object core.Object, name string, want any) {
	t.Helper()

	if got := r.read(t, object).Get(name); got != want {
		t.Errorf("scimtest: GET %s (RFC 7644 3.5.2): %s is %v, want %v", r.path(object), name, got, want)
	}
}
