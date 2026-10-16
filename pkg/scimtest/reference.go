package scimtest

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type link struct {
	source *resource
	target *resource
}

type referencing struct {
	link
	attribute *core.Attribute
	inverse   *core.Attribute
	optional  bool
}

func newReferencing(resources []*resource) []referencing {
	references := []referencing{}
	for _, source := range resources {
		for _, reference := range baseReferences(source) {
			if !isWritableList(reference.Attribute) {
				continue
			}
			for i, target := range targetsOf(resources, reference.Targets) {
				references = append(references, referencing{source: source, target: target, attribute: reference.Attribute, inverse: inverseOf(target, source), optional: i > 0})
			}
		}
	}
	return references
}

func (l link) newTarget(t *testing.T) string {
	t.Helper()

	_, created := l.target.create(t, newUnique())
	return idOf(created)
}

func (l link) patch(t *testing.T, source core.Object, operation patch.Operation) {
	t.Helper()
	l.source.expectPatched(t, l.source.client().Patch(t, l.source.path(source), ops(operation)), source)
}

func (r referencing) run(t *testing.T) {
	t.Helper()

	t.Run("stores the referenced values", r.checkStored)
	t.Run("adds values", r.checkAdd)
	t.Run("keeps the resource as is when adding a held value", r.checkAddHeld)
	t.Run("returns the values in a query", r.checkQueried)
	t.Run("projects the values with attributes and excludedAttributes", r.checkProjected)
	t.Run("finds the source by a value", r.checkFiltered)
	t.Run("finds the source by a value type", r.checkTypeFiltered)
	t.Run("removes a value by filter", r.checkRemoveFiltered)
	t.Run("replaces the values", r.checkReplace)
	t.Run("keeps the values a replace without a path leaves out", r.checkReplaceOthers)
	t.Run("replaces the values with PUT", r.checkPut)
	t.Run("removes every value without a filter", r.checkRemoveAll)
	t.Run("rejects a change to an immutable value", r.checkImmutable)
	t.Run("lists the source on the readOnly inverse", r.checkInverse)
	t.Run("types a nested source as indirect", r.checkIndirect)
	t.Run("drops a deleted target", r.checkDeletedTarget)
}

func (r referencing) checkStored(t *testing.T) {
	first := r.newTarget(t)

	source := r.newSource(t, first)
	r.source.expectValues(t, source, r.attribute.Name, first)
}

// RFC 7644 Section 3.4.2.5: attributes and excludedAttributes name a reference like any other attribute.
func (r referencing) checkProjected(t *testing.T) {
	source := r.newSource(t, r.newTarget(t))
	name := r.attribute.Name
	for param, want := range map[string]projection{
		attributesParam: {[]string{"id", name}, nil},
		excludedParam:   {[]string{"id"}, []string{name}},
	} {
		path := r.source.path(source) + "?" + url.Values{param: {name}}.Encode()
		want.expect(t, "GET "+path, get[core.Object](t, r.source.client(), path, "RFC 7644 3.4.2.5"))
	}
}

func (r referencing) checkAdd(t *testing.T) {
	r.patchable(t)
	first, second := r.newTarget(t), r.newTarget(t)
	source := r.newSource(t, first)

	r.patch(t, source, op(patch.OpAdd, r.attribute.Name, members(second)))
	r.source.expectValues(t, source, r.attribute.Name, first, second)
}

// RFC 7644 Section 3.5.2.1: if the target already contains the value, no changes SHOULD be made.
func (r referencing) checkAddHeld(t *testing.T) {
	r.source.suite.should(t, "RFC 7644 3.5.2.1 SHOULD: adding a held value succeeds and changes nothing")
	r.patchable(t)
	held := r.newTarget(t)
	source := r.newSource(t, held)
	before := metaOf(r.source.read(t, source))["lastModified"]

	r.patch(t, source, op(patch.OpAdd, r.attribute.Name, members(held)))
	r.source.expectValues(t, source, r.attribute.Name, held)
	r.source.expectLastModified(t, source, before)
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
		t.Errorf("scimtest: %s (RFC 7644 3.4.2): %s values %v, want %v", r.source.describe(query), r.attribute.Name, got, want)
	}
}

// RFC 7644 Sections 3.4.2.2 and 3.10: a filter matches a sub-attribute of any value, alone or in a value path, with names case insensitive.
func (r referencing) checkFiltered(t *testing.T) {
	supports(t, "filter", r.source.config.Filter.Supported)
	first, second := r.newTarget(t), r.newTarget(t)
	ids := []string{idOf(r.newSource(t, first)), idOf(r.newSource(t, second)), idOf(r.newSource(t))}
	name := r.attribute.Name
	urn := string(r.source.kind.Schema) + ":" + name

	for filter, want := range map[string][]string{
		name + ".value eq " + quoted(first):                       ids[:1],
		name + "[value eq " + quoted(first) + "]":                 ids[:1],
		"not (" + name + "[value eq " + quoted(first) + "])":      ids[1:],
		urn + ".value eq " + quoted(first):                        ids[:1],
		strings.ToLower(urn) + "[value eq " + quoted(first) + "]": ids[:1],
		name + " pr":            ids[:2],
		"not (" + name + " pr)": ids[2:],
	} {
		r.source.expectMatches(t, filter, ids, want)
	}
}

// RFC 7643 Section 4.1.2: a value path filter on "type" matches a canonical type of a value.
func (r referencing) checkTypeFiltered(t *testing.T) {
	supports(t, "filter", r.source.config.Filter.Supported)
	kind := string(r.target.kind.Name)
	if sub := r.attribute.SubAttribute(typeSub); sub == nil || !slices.Contains(sub.CanonicalValues, kind) {
		t.Skipf("n/a: %s.type has no canonical value %s", r.attribute.Name, kind)
	}
	body := r.sourceBody(t)
	body.Set(r.attribute.Name, []any{map[string]any{valueSub: r.newTarget(t), typeSub: kind}})
	ids := []string{idOf(r.source.seed(t, body)), idOf(r.newSource(t))}

	r.source.expectMatches(t, r.attribute.Name+"[type eq "+quoted(kind)+"]", ids, ids[:1])
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

// RFC 7644 Section 3.5.2.3: without a path, "value" SHALL contain the attributes to replace.
func (r referencing) checkReplaceOthers(t *testing.T) {
	r.patchable(t)
	sibling := r.source.attribute(t, "readWrite singular string attribute", patchable)
	ids := slices.Sorted(slices.Values([]string{r.newTarget(t), r.newTarget(t)}))
	source := r.newSource(t, ids...)

	res := r.source.client().Patch(t, r.source.path(source), ops(op(patch.OpReplace, "", map[string]string{sibling.Name: placeholder + " " + newUnique()})))
	r.source.expectPatched(t, res, source)
	if res.StatusCode == http.StatusOK {
		if got := valuesIn(Decode[core.Object](t, res), r.attribute.Name); !slices.Equal(got, ids) {
			t.Errorf("scimtest: PATCH %s (RFC 7644 3.5.2): %s values %v, want %v", r.source.path(source), r.attribute.Name, got, ids)
		}
	}
	r.source.expectValues(t, source, r.attribute.Name, ids...)
}

// RFC 7644 Section 3.5.1: readWrite values provided SHALL replace the existing values.
func (r referencing) checkPut(t *testing.T) {
	first, second := r.newTarget(t), r.newTarget(t)
	source := r.newSource(t, first)
	expectStatus(t, r.source.client().Put(t, r.source.path(source), r.sourceBody(t, second)), http.StatusOK, "RFC 7644 3.5.1")
	r.source.expectValues(t, source, r.attribute.Name, second)
}

// RFC 7644 Section 3.5.2.2: removing every value leaves the multi-valued attribute unassigned.
func (r referencing) checkRemoveAll(t *testing.T) {
	r.patchable(t)
	source := r.newSource(t, r.newTarget(t), r.newTarget(t))

	r.patch(t, source, op(patch.OpRemove, r.attribute.Name, nil))
	if read := r.source.read(t, source); !unassigned(read, r.attribute.Name) {
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

	read := r.target.read(t, target)
	if got := valuesIn(read, r.inverse.Name); !slices.Contains(got, idOf(source)) {
		t.Errorf("scimtest: GET %s (RFC 7643 4.1.2): %s %v, want %s", r.target.path(target), r.inverse.Name, got, idOf(source))
	}
	t.Run("types it direct", func(t *testing.T) {
		r.source.suite.should(t, "RFC 7643 4.1.2 groups.type is direct or indirect")
		r.expectType(t, read, idOf(source), "direct")
	})
	r.patch(t, source, op(patch.OpRemove, r.attribute.Name+"[value eq "+quoted(first)+"]", nil))
	if got := valuesIn(r.target.read(t, target), r.inverse.Name); slices.Contains(got, idOf(source)) {
		t.Errorf("scimtest: GET %s (RFC 7643 4.1.2): %s %v, want no %s", r.target.path(target), r.inverse.Name, got, idOf(source))
	}
}

// RFC 7643 Section 4.1.2: membership through a nested source is "indirect".
func (r referencing) checkIndirect(t *testing.T) {
	r.source.suite.should(t, "RFC 7643 4.1.2 a membership through a nested group is typed indirect")
	r.patchable(t)
	if r.inverse == nil || len(targetsOf([]*resource{r.source}, r.attribute.ReferenceTargets())) == 0 {
		t.Skipf("n/a: %s.%s cannot nest %s", r.source.kind.Name, r.attribute.Name, r.source.kind.Name)
	}
	first := r.newTarget(t)
	child := r.newSource(t, first)
	parent := r.newSource(t)
	r.patch(t, parent, op(patch.OpAdd, r.attribute.Name, members(idOf(child))))
	target := core.Object{"id": first}

	read := r.target.read(t, target)
	if elementWith(read, r.inverse.Name, idOf(parent)) == nil {
		t.Skipf("n/a: %s lists direct memberships only", r.inverse.Name)
	}
	r.expectType(t, read, idOf(parent), "indirect")
}

func (r referencing) expectType(t *testing.T, target core.Object, id, want string) {
	t.Helper()

	element := elementWith(target, r.inverse.Name, id)
	if kind, ok := element[typeSub]; ok && kind != want {
		t.Errorf("scimtest: GET %s (RFC 7643 4.1.2): %s %s has type %v, want %s", r.target.path(target), r.inverse.Name, id, kind, want)
	}
}

func (r referencing) checkDeletedTarget(t *testing.T) {
	r.source.suite.should(t, "RFC 7643 2.3.7 MAY: referential integrity drops a deleted target from referencing values")
	first, second := r.newTarget(t), r.newTarget(t)
	source := r.newSource(t, first, second)

	expectStatus(t, r.target.client().Delete(t, r.target.path(core.Object{"id": first})), http.StatusNoContent, "RFC 7644 3.6")
	r.source.expectValues(t, source, r.attribute.Name, second)
}

func (r referencing) newSource(t *testing.T, ids ...string) core.Object {
	t.Helper()

	body := r.sourceBody(t, ids...)
	if r.optional {
		return r.source.seed(t, body)
	}
	_, created := r.source.createBody(t, body)
	return created
}

func (r referencing) sourceBody(t *testing.T, ids ...string) core.Object {
	t.Helper()

	body := r.source.body(t, newUnique())
	body.Set(r.attribute.Name, members(ids...))
	return body
}

func targetsOf(resources []*resource, targets []core.ReferenceType) []*resource {
	found := []*resource{}
	for _, target := range targets {
		if i := slices.IndexFunc(resources, func(r *resource) bool { return r.kind.Name.Reference() == target }); i >= 0 {
			found = append(found, resources[i])
		}
	}
	return found
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
