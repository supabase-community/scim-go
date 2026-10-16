package scimtest

import (
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
	t.Run("rejects a duplicate unique value", r.checkUniqueness)
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
	_, created := r.create(t, unique)
	body := r.body(t, unique)
	o.set(body, marker)
	sibling := o.sibling()
	var want any
	if sibling != nil {
		want = r.siblingValue(o.attribute, sibling, created)
		objectOf(o.holder(body).Get(o.attribute.Name))[sibling.Name] = want
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
	r.suite.should(t, "RFC 7644 3.12 400 invalidValue for a PUT without a required attribute")
	unique := newUnique()
	body := r.body(t, unique)
	attribute := r.attribute(t, "required attribute in its fixture", required(body))
	_, created := r.createBody(t, body)
	body.Remove(attribute.Name)

	RequireError(t, r.client().Put(t, r.path(created), body), http.StatusBadRequest, scimerrors.InvalidValue)
}

func (r replacing) checkUniqueness(t *testing.T) {
	r.suite.should(t, "RFC 7644 Table 9 409 uniqueness for a PUT with another resource's unique value")
	taken := newUnique()
	body := r.body(t, taken)
	r.attribute(t, "unique attribute in its fixture", func(a *core.Attribute) bool { return isUnique(a) && body.Has(a.Name) })
	r.createBody(t, body)
	_, created := r.create(t, newUnique())

	RequireError(t, r.client().Put(t, r.path(created), r.body(t, taken)), http.StatusConflict, scimerrors.Uniqueness)
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

func (r replacing) siblingValue(parent, sub *core.Attribute, created core.Object) string {
	if ref := parent.SubAttribute("$ref"); sub.Name == valueSub && ref != nil && slices.Contains(ref.ReferenceTypes, r.kind.Name.Reference()) {
		return idOf(created)
	}
	return placeholder + " " + newUnique()
}
