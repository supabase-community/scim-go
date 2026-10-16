package scimtest

import (
	"maps"
	"net/http"
	"slices"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type replacing struct {
	*resource
}

func (r replacing) run(t *testing.T) {
	t.Helper()

	t.Run("replaces the resource", r.checkPut)
	t.Run("ignores a readOnly id", r.checkPutReadOnlyID)
	t.Run("ignores readOnly attributes", r.checkPutReadOnly)
	t.Run("rejects a changed immutable attribute", r.checkImmutable)
	t.Run("rejects a missing required attribute", r.checkRequired)
	t.Run("rejects a value of the wrong type", r.checkMistyped)
	t.Run("rejects a duplicate unique value", r.checkUniqueness)
	t.Run("accepts a repeated value that is not unique", r.checkNotUnique)
}

// RFC 7644 Section 3.5.1: readWrite values provided SHALL replace the existing values.
func (r replacing) checkPut(t *testing.T) {
	unique := newUnique()
	_, created := r.create(t, unique)
	body := r.body(t, unique)
	attribute := firstOf(r.schemas.Base().Attributes, patchable)
	want := placeholder + " " + newUnique()
	if attribute != nil {
		body.Set(attribute.Name, want)
	}

	res := r.client().Put(t, r.path(created), body)
	expectStatus(t, res, http.StatusOK, "RFC 7644 3.5.1")
	replaced := Decode[core.Object](t, res)
	if got := idOf(replaced); got != idOf(created) {
		t.Errorf("scimtest: PUT %s (RFC 7644 3.5.1): id %q, want %q", r.path(created), got, idOf(created))
	}
	r.expectLocated(t, res)
	if attribute == nil {
		return
	}
	for where, object := range map[string]core.Object{http.MethodPut: replaced, http.MethodGet: r.read(t, created)} {
		if got := object.Get(attribute.Name); got != want {
			t.Errorf("scimtest: %s %s (RFC 7644 3.5.1): %s is %v, want %v", where, r.path(created), attribute.Name, got, want)
		}
	}
}

// RFC 7644 Section 3.5.1: readOnly values provided SHALL be ignored.
func (r replacing) checkPutReadOnlyID(t *testing.T) {
	unique := newUnique()
	_, created := r.create(t, unique)
	body := r.body(t, unique)
	body.Set("id", "scimtest-readonly-"+newUnique())

	res := r.client().Put(t, r.path(created), body)
	expectStatus(t, res, http.StatusOK, "RFC 7644 3.5.1")
	for where, object := range map[string]core.Object{http.MethodPut: Decode[core.Object](t, res), http.MethodGet: r.read(t, created)} {
		if got := idOf(object); got != idOf(created) {
			t.Errorf("scimtest: %s %s (RFC 7644 3.5.1): id %q, want %q", where, r.path(created), got, idOf(created))
		}
	}
}

// RFC 7644 Section 3.5.1: readOnly values provided SHALL be ignored.
func (r replacing) checkPutReadOnly(t *testing.T) {
	for _, o := range readOnlyIn(t, r.resource) {
		t.Run(o.path(), func(t *testing.T) { r.putReadOnly(t, o) })
	}
}

func (r replacing) putReadOnly(t *testing.T, o readOnly) {
	t.Helper()

	marker := "scimtest-readonly-" + newUnique()
	unique := newUnique()
	sibling := o.sibling()
	body := r.body(t, unique)
	if sibling != nil {
		r.setSibling(t, o, sibling, body)
	}
	_, created := r.createBody(t, body)
	body = r.body(t, unique)
	o.set(body, marker)
	var want any
	if sibling != nil {
		want = r.setSibling(t, o, sibling, body)
	}
	body.Set(schemasAttr, r.schemaURIs(body))

	res := r.client().Put(t, r.path(created), body)
	expectStatus(t, res, http.StatusOK, "RFC 7644 3.5.1")
	for where, object := range map[string]core.Object{http.MethodPut: Decode[core.Object](t, res), http.MethodGet: r.read(t, created)} {
		expectIgnored(t, where+" "+r.path(created)+" (RFC 7644 3.5.1)", object, marker)
		if sibling == nil {
			continue
		}
		if got := o.subValue(object, sibling.Name); got != want {
			t.Errorf("scimtest: %s %s (RFC 7644 3.5.1): %s.%s is %v, want %v", where, r.path(created), o.attribute.Name, sibling.Name, got, want)
		}
	}
}

func (r replacing) checkRequired(t *testing.T) {
	r.suite.should(t, "RFC 7644 3.5.1 and Table 9 400 invalidValue for a PUT without a required attribute")
	unique := newUnique()
	body := r.body(t, unique)
	attribute := r.attribute(t, "required attribute in its fixture", required(body))
	_, created := r.createBody(t, body)
	body.Remove(attribute.Name)

	RequireError(t, r.client().Put(t, r.path(created), body), http.StatusBadRequest, scimerrors.InvalidValue)
}

func (r replacing) checkUniqueness(t *testing.T) {
	r.suite.should(t, "RFC 7644 3.3 and Table 8 409 uniqueness (RFC 7643 7 allows 400) for a PUT with another resource's unique value")
	taken := newUnique()
	body := r.body(t, taken)
	r.attribute(t, "unique attribute in its fixture", func(a *core.Attribute) bool { return isUnique(a) && body.Has(a.Name) })
	r.createBody(t, body)
	_, created := r.create(t, newUnique())

	res := r.client().Put(t, r.path(created), r.body(t, taken))
	if res.StatusCode == http.StatusBadRequest {
		RequireError(t, res, http.StatusBadRequest, "")
		return
	}
	RequireError(t, res, http.StatusConflict, scimerrors.Uniqueness)
}

func (r replacing) checkNotUnique(t *testing.T) {
	r.suite.should(t, "RFC 7643 7 a PUT may repeat another resource's uniqueness none value")
	value := placeholder + " " + newUnique()
	r.createBody(t, r.sharing(t, newUnique(), value))
	unique := newUnique()
	_, created := r.create(t, unique)

	expectStatus(t, r.client().Put(t, r.path(created), r.sharing(t, unique, value)), http.StatusOK, "RFC 7644 3.5.1")
}

func (r replacing) checkImmutable(t *testing.T) {
	r.suite.should(t, "RFC 7644 3.5.1 400 mutability for a changed immutable attribute")
	body := r.body(t, newUnique())
	attribute := r.attribute(t, "immutable attribute in its fixture", func(a *core.Attribute) bool {
		return a.Mutability == core.MutabilityImmutable && a.Type == core.TypeString && !a.MultiValued && body.Has(a.Name)
	})
	_, created := r.createBody(t, body)
	body.Set(attribute.Name, "scimtest "+newUnique())

	RequireError(t, r.client().Put(t, r.path(created), body), http.StatusBadRequest, scimerrors.Mutability)
}

func (r replacing) setSibling(t *testing.T, o readOnly, sibling *core.Attribute, body core.Object) string {
	t.Helper()

	value := r.siblingValue(t, o.attribute, sibling)
	holder := o.holder(body)
	parent := maps.Clone(objectOf(holder.Get(o.attribute.Name)))
	if parent == nil {
		parent = map[string]any{}
	}
	parent[sibling.Name] = value
	holder.Set(o.attribute.Name, parent)
	body.Set(schemasAttr, r.schemaURIs(body))
	return value
}

// RFC 7643 Section 4.3: "manager" references the "id" attribute of another User.
func (r replacing) siblingValue(t *testing.T, parent, sub *core.Attribute) string {
	t.Helper()

	if ref := parent.SubAttribute("$ref"); sub.Name == valueSub && ref != nil && slices.Contains(ref.ReferenceTypes, r.kind.Name.Reference()) {
		_, other := r.create(t, newUnique())
		return idOf(other)
	}
	return placeholder + " " + newUnique()
}

func (r replacing) checkMistyped(t *testing.T) {
	r.suite.should(t, "RFC 7644 Table 9 400 invalidValue for a value of the wrong type")
	unique := newUnique()
	_, created := r.create(t, unique)
	body := r.body(t, unique)
	r.mistype(t, body)

	expectTypeError(t, r.client().Put(t, r.path(created), body))
}
