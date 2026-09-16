package server

import (
	"net/http"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

func defaultServiceProviderConfig() *core.ServiceProviderConfig {
	return (&core.ServiceProviderConfig{
		Schemas:               []core.SchemaURI{core.SchemaServiceProviderConfig},
		AuthenticationSchemes: []*core.AuthenticationScheme{core.NewOAuthBearerToken()},
	}).Sorting().Filtering(protocol.DefaultLimits.MaxCount).Patching()
}

func mountDiscovery(mux *http.ServeMux, basePath string, schemas []*core.Schema, resourceTypes []*core.ResourceType, config *core.ServiceProviderConfig) {
	mux.HandleFunc("GET "+basePath+"/ServiceProviderConfig", func(w http.ResponseWriter, _ *http.Request) {
		_ = protocol.Send(w, http.StatusOK, config)
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
}
