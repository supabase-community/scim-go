package server

import (
	"fmt"
	"net/http"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
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

	// mountDiscovery(mux, basePath, schemas, resourceTypes, defaultServiceProviderConfig())
	mux.HandleFunc("GET "+basePath+"/ServiceProviderConfig", func(w http.ResponseWriter, _ *http.Request) {
		_ = protocol.Send(w, http.StatusOK, defaultServiceProviderConfig())
	})
	mux.HandleFunc("GET "+basePath+"/ResourceTypes", func(w http.ResponseWriter, _ *http.Request) {
		_ = protocol.Send(w, http.StatusOK, protocol.NewListResponse(1, len(resourceTypes), resourceTypes))
	})
	mux.HandleFunc("GET "+basePath+"/ResourceTypes/{id}", func(w http.ResponseWriter, r *http.Request) {
		for _, resourceType := range resourceTypes {
			if r.PathValue("id") == string(resourceType.ID) {
				_ = protocol.Send(w, http.StatusOK, resourceType)
				return
			}
		}
		_ = protocol.SendError(w, scimerrors.ErrNotFound("resource type not found"))
	})
	mux.HandleFunc("GET "+basePath+"/Schemas", func(w http.ResponseWriter, _ *http.Request) {
		_ = protocol.Send(w, http.StatusOK, protocol.NewListResponse(1, len(schemas), schemas))
	})
	mux.HandleFunc("GET "+basePath+"/Schemas/{id}", func(w http.ResponseWriter, r *http.Request) {
		for _, schema := range schemas {
			if r.PathValue("id") == string(schema.ID) {
				_ = protocol.Send(w, http.StatusOK, schema)
				return
			}
		}
		_ = protocol.SendError(w, scimerrors.ErrNotFound("schema not found"))
	})

	return &Server{mux: mux}, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func defaultServiceProviderConfig() *core.ServiceProviderConfig {
	return (&core.ServiceProviderConfig{
		Schemas:               []core.SchemaURI{core.SchemaServiceProviderConfig},
		AuthenticationSchemes: []*core.AuthenticationScheme{core.NewOAuthBearerToken()},
	}).Sorting().Filtering(protocol.DefaultLimits.MaxCount).Patching()
}
