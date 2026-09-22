package server

import (
	"net/http"

	"github.com/supabase-community/scim-go/pkg/core"
)

type Registration interface {
	resourceType(basePath string) *core.ResourceType
	schemas(basePath string) []*core.Schema
	mount(mux *http.ServeMux, basePath string)
}
