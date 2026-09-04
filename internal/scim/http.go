package scim

import (
	"net/http"
	"net/url"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

func listCollection[T any](w http.ResponseWriter, r *http.Request, resources []T) error {
	if rejected, err := rejectFilter(w, r); rejected {
		return err
	}
	return protocol.Send(w, http.StatusOK, protocol.NewListResponse(1, len(resources), resources))
}

func findByID[T core.Resource](w http.ResponseWriter, r *http.Request, resources []T) error {
	id := urlParam(r, "id")
	for _, resource := range resources {
		if resource.ResourceID() == id {
			return protocol.Send(w, http.StatusOK, resource)
		}
	}
	return notFound(w)
}

// located is a narrow duck-typed interface so locationOf can read a resource's
// Location() without importing core, avoiding a dependency back onto this package.
type located interface{ Location() string }

func locationOf(v any) string {
	if l, ok := v.(located); ok {
		return l.Location()
	}
	return ""
}

func notFound(w http.ResponseWriter) error {
	return protocol.SendError(w, protocol.ErrNotFound("Endpoint or resource does not exist"))
}

func rejectFilter(w http.ResponseWriter, r *http.Request) (bool, error) {
	if !r.URL.Query().Has("filter") {
		return false, nil
	}
	return true, protocol.SendError(w, protocol.ErrForbidden("Filtering is not supported on this endpoint"))
}

func urlParam(r *http.Request, key string) string {
	if decoded, err := url.PathUnescape(r.PathValue(key)); err == nil {
		return decoded
	}
	return r.PathValue(key)
}
