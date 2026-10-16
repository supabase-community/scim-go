package scimtest

import (
	"net/http"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
)

type identifying struct {
	resources []*resource
}

func (c identifying) run(t *testing.T) {
	t.Helper()

	t.Run("keeps every id unique across resource types", c.checkUniqueIDs)
}

// RFC 7643 Section 3.1: "id" MUST be unique across the service provider's entire set of resources.
func (c identifying) checkUniqueIDs(t *testing.T) {
	if len(c.resources) < 2 {
		t.Skip("n/a: fewer than two resource types")
	}
	owners := map[string]*resource{}
	for _, r := range c.resources {
		_, created := r.create(t, newUnique())
		id := idOf(created)
		if owner, taken := owners[id]; taken {
			t.Errorf("scimtest: POST %s (RFC 7643 3.1): id %q is also the id of a %s", r.kind.Endpoint, id, owner.kind.Name)
		}
		owners[id] = r
	}
	for id, owner := range owners {
		for _, other := range c.resources {
			if other != owner {
				RequireError(t, other.client().Get(t, other.path(core.Object{"id": id})), http.StatusNotFound, "")
			}
		}
	}
}
