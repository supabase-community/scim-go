package server

import (
	"net/http"

	"github.com/supabase-community/scim-go/pkg/core"
)

// Registration is the type-erased shape ResourceConfig[T] implements so New
// can mount resource types with different concrete T in one call.
type Registration interface {
	resourceType(basePath string) *core.ResourceType
	schemas() []*core.Schema
	mount(mux *http.ServeMux, basePath string)
}
