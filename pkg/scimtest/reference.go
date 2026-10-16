package scimtest

import (
	"net/http"
	"slices"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type referencing struct {
	source    *resource
	target    *resource
	attribute *core.Attribute
	inverse   *core.Attribute
}

func newReferencing(resources []*resource) []referencing {
	references := []referencing{}
	for _, source := range resources {
		for _, reference := range baseReferences(source) {
			if !isWritableList(reference.Attribute) {
				continue
			}
			if target := targetOf(resources, reference.Targets); target != nil {
				references = append(references, referencing{source: source, target: target, attribute: reference.Attribute, inverse: inverseOf(target, source)})
			}
		}
	}
	return references
}

func (r referencing) run(t *testing.T) {
	t.Helper()

	t.Run("stores the referenced values", r.checkStored)
	t.Run("adds values", r.checkAdd)
	t.Run("returns the values in a query", r.checkQueried)
	t.Run("removes a value by filter", r.checkRemoveFiltered)
	t.Run("replaces the values", r.checkReplace)
	t.Run("removes every value without a filter", r.checkRemoveAll)
	t.Run("rejects a change to an immutable value", r.checkImmutable)
	t.Run("lists the source on the readOnly inverse", r.checkInverse)
	t.Run("drops a deleted target", r.checkDeletedTarget)
}

func (r referencing) checkStored(t *testing.T) {
	first := r.newTarget(t)

	source := r.newSource(t, first)
	r.source.expectValues(t, source, r.attribute.Name, first)
}

func (r referencing) checkAdd(t *testing.T) {
	r.patchable(t)
	first, second := r.newTarget(t), r.newTarget(t)
	source := r.newSource(t, first)

	r.patch(t, source, op(patch.OpAdd, r.attribute.Name, members(second)))
	r.source.expectValues(t, source, r.attribute.Name, first, second)
}

func (r referencing) checkQueried(t *testing.T) {
	supports(t, "filter", r.source.config.Filter.Supported)
	ids := []string{r.newTarget(t), r.newTarget(t)}
	source := r.newSource(t, ids...)
	query := byID(idOf(source))

	page := r.source.list(t, query)
	if len(page.Resources) != 1 {
		t.Fatalf("scimtest: %s (RFC 7644 3.4.2): %d resources, want 1", r.source.describe(query), len(page.Resources))
	}
	if got, want := valuesIn(page.Resources[0], r.attribute.Name), slices.Sorted(slices.Values(ids)); !slices.Equal(got, want) {
		t.Errorf("scimtest: %s (RFC 7643 2.3.7): %s values %v, want %v", r.source.describe(query), r.attribute.Name, got, want)
	}
}

func (r referencing) checkRemoveFiltered(t *testing.T) {
	r.patchable(t)
	first, second := r.newTarget(t), r.newTarget(t)
	source := r.newSource(t, first, second)

	r.patch(t, source, op(patch.OpRemove, r.attribute.Name+"[value eq "+quoted(first)+"]", nil))
	r.source.expectValues(t, source, r.attribute.Name, second)
}

func (r referencing) checkReplace(t *testing.T) {
	r.patchable(t)
	first, second := r.newTarget(t), r.newTarget(t)
	source := r.newSource(t, first)

	r.patch(t, source, op(patch.OpReplace, r.attribute.Name, members(second)))
	r.source.expectValues(t, source, r.attribute.Name, second)
}

// RFC 7644 Section 3.5.2.2: removing every value leaves the multi-valued attribute unassigned.
func (r referencing) checkRemoveAll(t *testing.T) {
	r.patchable(t)
	source := r.newSource(t, r.newTarget(t), r.newTarget(t))

	r.patch(t, source, op(patch.OpRemove, r.attribute.Name, nil))
	if read := r.source.read(t, source); read.Has(r.attribute.Name) {
		t.Errorf("scimtest: PATCH %s (RFC 7644 3.5.2.2): %s is %v, want unassigned", r.source.path(source), r.attribute.Name, read.Get(r.attribute.Name))
	}
}

// RFC 7644 Section 3.5.2: an operation MUST be compatible with the attribute's mutability.
func (r referencing) checkImmutable(t *testing.T) {
	r.patchable(t)
	if r.attribute.SubAttribute(valueSub).Mutability != core.MutabilityImmutable {
		t.Skipf("n/a: %s.value is not immutable", r.attribute.Name)
	}
	first, second := r.newTarget(t), r.newTarget(t)
	source := r.newSource(t, first)
	path := r.attribute.Name + "[value eq " + quoted(first) + "].value"

	RequireError(t, r.source.client().Patch(t, r.source.path(source), ops(op(patch.OpReplace, path, second))), http.StatusBadRequest, scimerrors.Mutability)
}

// RFC 7643 Section 4.1.2: "groups" is readOnly and changes through the membership of the referencing resource.
func (r referencing) checkInverse(t *testing.T) {
	r.patchable(t)
	if r.inverse == nil {
		t.Skipf("n/a: %s has no readOnly attribute referencing %s", r.target.kind.Name, r.source.kind.Name)
	}
	first := r.newTarget(t)
	source := r.newSource(t, first)
	target := core.Object{"id": first}

	if got := valuesIn(r.target.read(t, target), r.inverse.Name); !slices.Contains(got, idOf(source)) {
		t.Errorf("scimtest: GET %s (RFC 7643 4.1.2): %s %v, want %s", r.target.path(target), r.inverse.Name, got, idOf(source))
	}
	r.patch(t, source, op(patch.OpRemove, r.attribute.Name+"[value eq "+quoted(first)+"]", nil))
	if got := valuesIn(r.target.read(t, target), r.inverse.Name); slices.Contains(got, idOf(source)) {
		t.Errorf("scimtest: GET %s (RFC 7643 4.1.2): %s %v, want no %s", r.target.path(target), r.inverse.Name, got, idOf(source))
	}
}

func (r referencing) checkDeletedTarget(t *testing.T) {
	r.source.suite.should(t, "RFC 7644 3.6 a deleted resource is omitted from the values that reference it")
	first, second := r.newTarget(t), r.newTarget(t)
	source := r.newSource(t, first, second)

	expectStatus(t, r.target.client().Delete(t, r.target.path(core.Object{"id": first})), http.StatusNoContent, "RFC 7644 3.6")
	r.source.expectValues(t, source, r.attribute.Name, second)
}

func (r referencing) patch(t *testing.T, source core.Object, operation patch.Operation) {
	t.Helper()
	r.source.expectPatched(t, r.source.client().Patch(t, r.source.path(source), ops(operation)), source)
}

func (r referencing) newTarget(t *testing.T) string {
	t.Helper()

	_, created := r.target.create(t, newUnique())
	return idOf(created)
}

func (r referencing) newSource(t *testing.T, ids ...string) core.Object {
	t.Helper()

	body := r.source.body(t, newUnique())
	body.Set(r.attribute.Name, members(ids...))
	_, created := r.source.createBody(t, body)
	return created
}

func targetOf(resources []*resource, targets []core.ReferenceType) *resource {
	for _, target := range targets {
		if i := slices.IndexFunc(resources, func(r *resource) bool { return r.kind.Name.Reference() == target }); i >= 0 {
			return resources[i]
		}
	}
	return nil
}

func inverseOf(target, source *resource) *core.Attribute {
	for _, reference := range baseReferences(target) {
		if reference.MultiValued && isReadOnly(reference.Attribute) && slices.Contains(reference.Targets, source.kind.Name.Reference()) {
			return reference.Attribute
		}
	}
	return nil
}

func baseReferences(r *resource) []core.ReferenceAttribute {
	return core.Schemas{r.schemas.Base()}.ReferenceAttributes()
}
