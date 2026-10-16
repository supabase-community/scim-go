package scimtest

import (
	"net/http"
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
	t.Run("ignores readOnly attributes", r.checkPutReadOnly)
	t.Run("rejects a changed immutable attribute", r.checkImmutable)
}

func (r replacing) checkPut(t *testing.T) {
	unique := newUnique()
	_, created := r.create(t, unique)

	res := r.client().Put(t, r.path(created), r.body(t, unique))
	expectStatus(t, res, http.StatusOK, "RFC 7644 3.5.1")
	if got := idOf(Decode[core.Object](t, res)); got != idOf(created) {
		t.Errorf("scimtest: PUT %s (RFC 7644 3.5.1): id %q, want %q", r.path(created), got, idOf(created))
	}
	r.expectLocated(t, res)
}

func (r replacing) checkPutReadOnly(t *testing.T) {
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
