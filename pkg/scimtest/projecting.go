package scimtest

import (
	"net/http"
	"slices"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
)

type projecting struct {
	*resource
}

func (r projecting) run(t *testing.T) {
	t.Helper()

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
	for _, tr := range transports {
		t.Run("never excludes id from a query over "+tr.name, func(t *testing.T) { r.checkListedID(t, tr, idOf(created)) })
	}
}

// RFC 7643 Section 3.1: "id" is returned "always", so a query cannot exclude it.
func (r projecting) checkListedID(t *testing.T, tr transport, id string) {
	t.Helper()

	supports(t, "filter", r.config.Filter.Supported)
	query := byID(id)
	query.Set("excludedAttributes", "id")

	over := r.over(t, tr)
	if got := idsOf(over.list(t, query).Resources); !slices.Equal(got, []string{id}) {
		t.Errorf("scimtest: %s (RFC 7644 3.4.2.5): ids %v, want [%s]", over.describe(query), got, id)
	}
}

func (r projecting) expectAttributes(t *testing.T, path string, present, absent []string) {
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
