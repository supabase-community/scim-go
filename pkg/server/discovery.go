package server

import (
	"net/http"

	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

func (s *Server) listResourceTypes(w http.ResponseWriter, r *http.Request) error {
	if err := rejectFilter(r); err != nil {
		return protocol.SendError(w, err)
	}
	return protocol.Send(w, http.StatusOK, protocol.NewListResponse(1, len(s.resourceTypes), s.resourceTypes))
}

func (s *Server) listSchemas(w http.ResponseWriter, r *http.Request) error {
	if err := rejectFilter(r); err != nil {
		return protocol.SendError(w, err)
	}
	return protocol.Send(w, http.StatusOK, protocol.NewListResponse(1, len(s.schemas), s.schemas))
}

func (s *Server) resourceTypeByID(w http.ResponseWriter, r *http.Request) error {
	for _, resourceType := range s.resourceTypes {
		if r.PathValue("id") == string(resourceType.ID) {
			return protocol.Send(w, http.StatusOK, resourceType)
		}
	}
	return protocol.SendError(w, scimerrors.ErrNotFound("resource type not found"))
}

func (s *Server) schemaByID(w http.ResponseWriter, r *http.Request) error {
	for _, schema := range s.schemas {
		if r.PathValue("id") == string(schema.ID) {
			return protocol.Send(w, http.StatusOK, schema)
		}
	}
	return protocol.SendError(w, scimerrors.ErrNotFound("schema not found"))
}

func (s *Server) serviceProviderConfig(w http.ResponseWriter, _ *http.Request) error {
	return protocol.Send(w, http.StatusOK, s.config)
}

// me declines the "/Me" alias, per RFC 7644, Section 3.11.
func me(w http.ResponseWriter, _ *http.Request) error {
	return protocol.SendError(w, scimerrors.ErrNotImplemented(`"/Me" is not supported`))
}

// rejectFilter forbids "filter" on /ResourceTypes and /Schemas, per RFC 7644, Section 4.
func rejectFilter(r *http.Request) error {
	if r.URL.Query().Get("filter") != "" {
		return scimerrors.ErrForbidden(`"filter" is not supported on this endpoint`)
	}
	return nil
}
