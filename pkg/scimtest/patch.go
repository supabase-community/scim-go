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
	t.Run("replaces a boolean", r.checkReplaceBoolean)
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
	t.Run("applies several operations in order", r.checkOrder)
	t.Run("keeps a single primary value", r.checkPrimary)
	t.Run("adds the extension schema with an extension attribute", r.checkExtension)
	t.Run("rejects an unknown op", r.checkUnknownOp)
	t.Run("rejects an unknown path", r.checkUnknownPath)
	t.Run("rejects a readOnly id", r.checkReadOnly)
	t.Run("never changes a readOnly attribute", r.checkReadOnlyKept)
	t.Run("rejects a body without operations", r.checkEmpty)
	t.Run("returns 200 with the requested attributes", r.checkProjected)
	t.Run("replaces attributes without a path", r.checkReplaceWithoutPath)
	t.Run("replaces the values a value filter matches", r.checkReplaceFiltered)
	t.Run("replaces a sub-attribute of the values a value filter matches", r.checkReplaceFilteredSub)
	t.Run("keeps the sub-attributes a complex replace leaves out", r.checkReplaceComplex)
	t.Run("unassigns a removed sub-attribute", r.checkRemoveSub)
}

func (r patching) checkReplaceByPath(t *testing.T) {
	if r.suite.unsupported(t, "patch", r.config.Patch.Supported) {
		r.suite.should(t, "RFC 7644 3.12 Table 8 501 for an unsupported PATCH")
		_, created := r.create(t, newUnique())
		res := r.client().Patch(t, r.path(created), ops(op(patch.OpReplace, "externalId", placeholder)))
		expectStatus(t, res, http.StatusNotImplemented, "RFC 7644 3.12 Table 8")
		return
	}
	attribute := firstOf(r.schemas.Base().Attributes, patchable)
	if attribute == nil {
		r.suite.todo(t, "%s has no readWrite singular string attribute to PATCH", r.kind.Name)
	}
	_, created := r.create(t, newUnique())
	want := "scimtest " + newUnique()
	r.expectPatched(t, r.client().Patch(t, r.path(created), ops(op(patch.OpReplace, attribute.Name, want))), created)
	r.expectValue(t, created, attribute.Name, want)
}

func (r patching) checkReplaceBoolean(t *testing.T) {
	supports(t, "patch", r.config.Patch.Supported)
	attribute := r.attribute(t, "readWrite singular boolean attribute", isWritableBoolean)
	_, created := r.create(t, newUnique())

	for _, want := range []bool{false, true} {
		r.expectPatched(t, r.client().Patch(t, r.path(created), ops(op(patch.OpReplace, attribute.Name, want))), created)
		r.expectValue(t, created, attribute.Name, want)
	}
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

// RFC 7644 Section 3.5.2.3: a value selection filter without a match SHALL fail with 400 noTarget.
func (r patching) checkNoMatch(t *testing.T) {
	created, _ := r.patchTarget(t)
	attribute := r.attribute(t, "readWrite multi-valued complex attribute with a value", func(a *core.Attribute) bool {
		return a.MultiValued && a.Type == core.TypeComplex && a.Mutability == core.MutabilityReadWrite && a.SubAttributes.Lookup(valueSub) != nil
	})
	selector := attribute.Name + `[value eq "scimtest-none"]`

	for path, value := range map[string]any{selector: map[string]string{valueSub: placeholder}, selector + "." + valueSub: placeholder} {
		RequireError(t, r.client().Patch(t, r.path(created), ops(op(patch.OpReplace, path, value))), http.StatusBadRequest, scimerrors.NoTarget)
	}
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
	r.suite.should(t, "RFC 7644 3.5.2 and Table 8 400 for a PATCH body without the PatchOp schema")
	created, attribute := r.patchTarget(t)
	body := ops(op(patch.OpReplace, attribute.Name, placeholder))
	body.Schemas = nil

	RequireError(t, r.client().Patch(t, r.path(created), body), http.StatusBadRequest, "")
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
	r.expectLastModified(t, created, metaOf(created)["lastModified"])
}

func (r patching) createValued(t *testing.T, attribute *core.Attribute, value string) core.Object {
	t.Helper()

	body := r.body(t, newUnique())
	body.Set(attribute.Name, []any{map[string]any{valueSub: value}})
	_, created := r.createBody(t, body)
	return created
}

// RFC 7644 Section 3.5.2.2: a multi-valued attribute without values SHALL be considered unassigned.
func (r patching) checkValues(t *testing.T) {
	attribute := r.valued(t)
	unique := newUnique()
	first, second := email("first", unique), email("second", unique)
	created := r.createValued(t, attribute, first)

	r.expectPatched(t, r.client().Patch(t, r.path(created), ops(op(patch.OpAdd, attribute.Name, []any{map[string]any{valueSub: second}}))), created)
	r.expectValues(t, created, attribute.Name, first, second)
	r.expectPatched(t, r.client().Patch(t, r.path(created), ops(op(patch.OpRemove, attribute.Name+"[value eq "+quoted(second)+"]", nil))), created)
	r.expectValues(t, created, attribute.Name, first)
	r.expectPatched(t, r.client().Patch(t, r.path(created), ops(op(patch.OpRemove, attribute.Name+"[value eq "+quoted(first)+"]", nil))), created)
	if read := r.read(t, created); !unassigned(read, attribute.Name) {
		t.Errorf("scimtest: PATCH %s (RFC 7644 3.5.2.2): %s is %v, want unassigned", r.path(created), attribute.Name, read.Get(attribute.Name))
	}
}

// RFC 7644 Section 3.5.2: operations are applied sequentially in the order they appear.
func (r patching) checkOrder(t *testing.T) {
	attribute := r.valued(t)
	unique := newUnique()
	first, second := email("first", unique), email("second", unique)
	created := r.createValued(t, attribute, first)

	r.expectPatched(t, r.client().Patch(t, r.path(created), ops(
		op(patch.OpAdd, attribute.Name, []any{map[string]any{valueSub: second}}),
		op(patch.OpRemove, attribute.Name+"[value eq "+quoted(second)+"]", nil),
	)), created)
	r.expectValues(t, created, attribute.Name, first)
}

// RFC 7644 Section 3.5.2: setting "primary" to true SHALL set "primary" to false for the other values.
func (r patching) checkPrimary(t *testing.T) {
	attribute := r.valued(t)
	if attribute.SubAttributes.Lookup(primarySub) == nil {
		t.Skipf("n/a: %s has no primary", attribute.Name)
	}
	unique := newUnique()
	first, second := email("first", unique), email("second", unique)
	body := r.body(t, newUnique())
	body.Set(attribute.Name, []any{map[string]any{valueSub: first, primarySub: true}})
	_, created := r.createBody(t, body)

	r.expectPatched(t, r.client().Patch(t, r.path(created), ops(op(patch.OpAdd, attribute.Name, []any{map[string]any{valueSub: second, primarySub: true}}))), created)
	primaries := []any{}
	for _, element := range elementsOf(r.read(t, created).Get(attribute.Name)) {
		if element[primarySub] == true {
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
	extension, attribute := r.extended(t, patchable)
	_, created := r.create(t, newUnique())
	path := string(extension.ID) + ":" + attribute.Name
	want := placeholder + " " + newUnique()

	r.expectPatched(t, r.client().Patch(t, r.path(created), ops(op(patch.OpReplace, path, want))), created)
	read := r.read(t, created)
	if raw, _ := json.Marshal(read.Get(schemasAttr)); !strings.Contains(string(raw), string(extension.ID)) {
		t.Errorf("scimtest: PATCH %s (RFC 7644 3.5.2): schemas %s, want %s", r.path(created), raw, extension.ID)
	}
	if got := core.Object(objectOf(read.Get(string(extension.ID)))).Get(attribute.Name); got != want {
		t.Errorf("scimtest: PATCH %s (RFC 7644 3.5.2): %s is %v, want %v", r.path(created), path, got, want)
	}
}

// RFC 7644 Table 8: 400 when the request violates the schema, and 3.5.2 limits "op" to add, remove or replace.
func (r patching) checkUnknownOp(t *testing.T) {
	created, attribute := r.patchTarget(t)
	body := ops(op(patch.OpReplace, attribute.Name, placeholder))
	body.Operations[0].Op = "scimtest"

	RequireError(t, r.client().Patch(t, r.path(created), body), http.StatusBadRequest, "")
}

func (r patching) checkUnknownPath(t *testing.T) {
	r.suite.should(t, "RFC 7644 Table 9 400 invalidPath or noTarget for a path to an undefined attribute")
	created, _ := r.patchTarget(t)

	body := RequireError(t, r.client().Patch(t, r.path(created), ops(op(patch.OpReplace, "scimtest"+newUnique(), placeholder))), http.StatusBadRequest, "")
	if body.ScimType != scimerrors.InvalidPath && body.ScimType != scimerrors.NoTarget {
		t.Errorf("scimtest: PATCH %s (RFC 7644 Table 9): scimType %q, want invalidPath or noTarget", r.path(created), body.ScimType)
	}
}

// RFC 7644 Section 3.5.2: a client MUST NOT modify a readOnly attribute, so its stored value is unchanged.
func (r patching) checkReadOnlyKept(t *testing.T) {
	supports(t, "patch", r.config.Patch.Supported)
	for _, o := range readOnlyIn(t, r.resource) {
		marker := "scimtest-readonly-" + newUnique()
		fragment := core.Object{}
		o.set(fragment, marker)
		for name, operation := range map[string]patch.Operation{
			"a path":  op(patch.OpReplace, o.path(), marker),
			"no path": op(patch.OpReplace, "", fragment),
		} {
			t.Run(o.path()+" with "+name, func(t *testing.T) {
				_, created := r.create(t, newUnique())
				res := r.client().Patch(t, r.path(created), ops(operation))
				if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNoContent && res.StatusCode != http.StatusBadRequest {
					t.Fatalf("scimtest: PATCH %s (RFC 7644 3.5.2): status %d, want 200, 204 or 400", r.path(created), res.StatusCode)
				}
				expectIgnored(t, "GET "+r.path(created)+" (RFC 7644 3.5.2)", r.read(t, created), marker)
			})
		}
	}
}

// RFC 7644 Section 3.5.2: an operation incompatible with readOnly mutability SHALL return an error, and Table 9 names it "mutability" with 400.
func (r patching) checkReadOnly(t *testing.T) {
	created, _ := r.patchTarget(t)

	RequireError(t, r.client().Patch(t, r.path(created), ops(op(patch.OpReplace, "id", "scimtest-"+newUnique()))), http.StatusBadRequest, scimerrors.Mutability)
}

func (r patching) checkEmpty(t *testing.T) {
	r.suite.should(t, "RFC 7644 3.5.2 and Table 8 400 for a PATCH body without Operations")
	created, _ := r.patchTarget(t)

	RequireError(t, r.client().Patch(t, r.path(created), ops()), http.StatusBadRequest, "")
}

// RFC 7644 Section 3.5.2: the server MUST return 200 OK if the "attributes" parameter is specified.
func (r patching) checkProjected(t *testing.T) {
	created, attribute := r.patchTarget(t)
	other := firstOf(r.schemas.Base().Attributes, func(a *core.Attribute) bool {
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

// RFC 7644 Section 3.5.2.3: without a path, "value" SHALL contain the attributes to replace.
func (r patching) checkReplaceWithoutPath(t *testing.T) {
	created, attribute := r.patchTarget(t)
	want := placeholder + " " + newUnique()

	r.expectPatched(t, r.client().Patch(t, r.path(created), ops(op(patch.OpReplace, "", map[string]string{attribute.Name: want}))), created)
	r.expectValue(t, created, attribute.Name, want)
}

// RFC 7644 Section 3.5.2.3: all values matching a value selection filter SHALL be replaced.
func (r patching) checkReplaceFiltered(t *testing.T) {
	attribute, created, first, second := r.twoValues(t)
	third := email("third", newUnique())
	path := attribute.Name + "[value eq " + quoted(first) + "]"

	r.expectPatched(t, r.client().Patch(t, r.path(created), ops(op(patch.OpReplace, path, map[string]any{valueSub: third}))), created)
	r.expectValues(t, created, attribute.Name, second, third)
}

// RFC 7644 Section 3.5.2.3: the matching sub-attribute of all matching records is replaced.
func (r patching) checkReplaceFilteredSub(t *testing.T) {
	attribute, created, first, second := r.twoValues(t)
	third := email("third", newUnique())
	path := attribute.Name + "[value eq " + quoted(first) + "]." + valueSub

	r.expectPatched(t, r.client().Patch(t, r.path(created), ops(op(patch.OpReplace, path, third))), created)
	r.expectValues(t, created, attribute.Name, second, third)
}

// RFC 7644 Section 3.5.2.3: sub-attributes not specified in "value" are left unchanged.
func (r patching) checkReplaceComplex(t *testing.T) {
	supports(t, "patch", r.config.Patch.Supported)
	attribute, created, kept, replaced := r.complexTarget(t)
	want := placeholder + " " + newUnique()

	r.expectPatched(t, r.client().Patch(t, r.path(created), ops(op(patch.OpReplace, attribute.Name, map[string]any{replaced: want}))), created)
	got := core.Object(objectOf(r.read(t, created).Get(attribute.Name)))
	if got.Get(replaced) != want || got.Get(kept) != objectOf(created.Get(attribute.Name))[kept] {
		t.Errorf("scimtest: PATCH %s (RFC 7644 3.5.2.3): %s is %v, want %s replaced and %s kept", r.path(created), attribute.Name, map[string]any(got), replaced, kept)
	}
}

// RFC 7644 Section 3.5.2.2: a removed single-value attribute SHALL be considered unassigned.
func (r patching) checkRemoveSub(t *testing.T) {
	supports(t, "patch", r.config.Patch.Supported)
	attribute, created, kept, removed := r.complexTarget(t)

	r.expectPatched(t, r.client().Patch(t, r.path(created), ops(op(patch.OpRemove, attribute.Name+"."+removed, nil))), created)
	got := core.Object(objectOf(r.read(t, created).Get(attribute.Name)))
	if !unassigned(got, removed) || unassigned(got, kept) {
		t.Errorf("scimtest: PATCH %s (RFC 7644 3.5.2.2): %s is %v, want %s unassigned and %s kept", r.path(created), attribute.Name, map[string]any(got), removed, kept)
	}
}

func (r patching) twoValues(t *testing.T) (*core.Attribute, core.Object, string, string) {
	t.Helper()

	attribute := r.valued(t)
	unique := newUnique()
	first, second := email("first", unique), email("second", unique)
	body := r.body(t, newUnique())
	body.Set(attribute.Name, []any{map[string]any{valueSub: first}, map[string]any{valueSub: second}})
	_, created := r.createBody(t, body)
	return attribute, created, first, second
}

func (r patching) valued(t *testing.T) *core.Attribute {
	t.Helper()

	supports(t, "patch", r.config.Patch.Supported)
	return r.attribute(t, "readWrite multi-valued complex attribute with a value", func(a *core.Attribute) bool {
		return a.MultiValued && a.Type == core.TypeComplex && a.Mutability == core.MutabilityReadWrite && a.SubAttributes.Lookup(valueSub) != nil && a.SubAttributes.Lookup(valueSub).Mutability == core.MutabilityReadWrite
	})
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
