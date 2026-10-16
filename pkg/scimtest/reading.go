package scimtest

import (
	"net/http"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
)

type reading struct {
	*resource
}

func (r reading) run(t *testing.T) {
	t.Helper()

	t.Run("returns the resource", r.checkGet)
	t.Run("returns 404 for an unknown id", r.checkUnknown)
	t.Run("never returns a returned:never attribute", r.checkNever)
}

func (r reading) checkGet(t *testing.T) {
	_, created := r.create(t, newUnique())

	res := r.client().Get(t, r.path(created))
	expectStatus(t, res, http.StatusOK, "RFC 7644 3.4.1")
	if got := idOf(Decode[core.Object](t, res)); got != idOf(created) {
		t.Errorf("scimtest: GET %s (RFC 7644 3.4.1): id %q, want %q", r.path(created), got, idOf(created))
	}
	r.expectLocated(t, res)
}

func (r reading) checkUnknown(t *testing.T) {
	RequireError(t, r.client().Get(t, r.kind.Endpoint+"/scimtest-"+newUnique()), http.StatusNotFound, "")
}

func (r reading) checkNever(t *testing.T) {
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
