package server

import (
	"fmt"
	"net/http"

	"github.com/supabase-community/scim-go/pkg/core"
)

type Server struct {
	mux *http.ServeMux
}

func New(basePath string, resources ...Registration) (*Server, error) {
	mux := http.NewServeMux()

	seen := make(map[string]bool)
	var resourceTypes []*core.ResourceType
	var schemas []*core.Schema
	for _, resource := range resources {
		resourceType := resource.resourceType(basePath)
		if seen[resourceType.Endpoint] {
			return nil, fmt.Errorf("scim: resource type %q already registered at %s", resourceType.Name, resourceType.Endpoint)
		}
		seen[resourceType.Endpoint] = true

		resource.mount(mux, basePath)
		resourceTypes = append(resourceTypes, resourceType)
		schemas = append(schemas, resource.schemas()...)
	}

	mountDiscovery(mux, basePath, schemas, resourceTypes, defaultServiceProviderConfig())

	return &Server{mux: mux}, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}
