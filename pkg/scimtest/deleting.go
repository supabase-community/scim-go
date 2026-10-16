package scimtest

import (
	"net/http"
	"testing"

	"github.com/supabase-community/scim-go/pkg/patch"
)

type deleting struct {
	*resource
}

func (r deleting) run(t *testing.T) {
	t.Helper()

	t.Run("returns 404 for every operation afterwards", r.checkDeleted)
	t.Run("omits the resource from queries", r.checkDeletedUnlisted)
	t.Run("accepts a new resource with the same unique values", r.checkDeletedReused)
}

// RFC 7644 Section 3.6: the service provider MUST return 404 for all operations associated with a deleted resource.
func (r deleting) checkDeleted(t *testing.T) {
	unique := newUnique()
	_, created := r.create(t, unique)
	path := r.path(created)

	expectStatus(t, r.client().Delete(t, path), http.StatusNoContent, "RFC 7644 3.6")
	RequireError(t, r.client().Get(t, path), http.StatusNotFound, "")
	RequireError(t, r.client().Put(t, path, r.body(t, unique)), http.StatusNotFound, "")
	if attribute := firstOf(r.schemas.Base().Attributes, patchable); attribute != nil && r.config.Patch.Supported {
		RequireError(t, r.client().Patch(t, path, ops(op(patch.OpReplace, attribute.Name, placeholder))), http.StatusNotFound, "")
	}
	RequireError(t, r.client().Delete(t, path), http.StatusNotFound, "")
}

func (r deleting) checkDeletedUnlisted(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	_, created := r.create(t, newUnique())
	expectStatus(t, r.client().Delete(t, r.path(created)), http.StatusNoContent, "RFC 7644 3.6")

	query := byID(idOf(created))
	if got := idsOf(r.list(t, query).Resources); len(got) != 0 {
		t.Errorf("scimtest: %s (RFC 7644 3.6): ids %v, want none", r.describe(query), got)
	}
}

func (r deleting) checkDeletedReused(t *testing.T) {
	r.suite.should(t, "RFC 7644 3.6 a deleted resource is not considered in conflict calculation")
	unique := newUnique()
	_, created := r.create(t, unique)
	expectStatus(t, r.client().Delete(t, r.path(created)), http.StatusNoContent, "RFC 7644 3.6")

	r.create(t, unique)
}
