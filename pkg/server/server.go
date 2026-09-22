package server

import (
	"net/http"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type Server struct {
	mux           *http.ServeMux
	handler       http.Handler
	basePath      string
	resourceTypes []*core.ResourceType
	schemas       []*core.Schema
	config        *core.ServiceProviderConfig
	errorHandler  func(error)
}

func New(basePath string) *Server {
	mux := http.NewServeMux()
	s := &Server{
		mux:          mux,
		handler:      mux,
		basePath:     basePath,
		config:       core.NewServiceProviderConfig().Sorting().Filtering(protocol.DefaultLimits.MaxCount).Patching().Versioning(),
		errorHandler: func(error) {},
	}

	mux.HandleFunc("GET "+basePath+"/ServiceProviderConfig", s.handle(s.serviceProviderConfig))
	mux.HandleFunc("GET "+basePath+"/ResourceTypes", s.handle(s.listResourceTypes))
	mux.HandleFunc("GET "+basePath+"/ResourceTypes/{id}", s.handle(s.resourceTypeByID))
	mux.HandleFunc("GET "+basePath+"/Schemas", s.handle(s.listSchemas))
	mux.HandleFunc("GET "+basePath+"/Schemas/{id}", s.handle(s.schemaByID))

	return s
}

func (s *Server) WithResource(resource Registration) *Server {
	s.resourceTypes = append(s.resourceTypes, resource.resourceType(s.basePath))
	s.schemas = append(s.schemas, resource.schemas(s.basePath)...)
	resource.mount(s.mux, s.basePath)
	return s
}

func (s *Server) WithErrorHandler(fn func(error)) *Server {
	s.errorHandler = fn
	return s
}

// WithAuthentication advertises scheme in ServiceProviderConfig, per RFC 7643, Section 5,
// and enforces it by wrapping every request with middleware.
func (s *Server) WithAuthentication(scheme *core.AuthenticationScheme, middleware func(http.Handler) http.Handler) *Server {
	s.config.Authentication(scheme)
	s.handler = middleware(s.handler)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

func (s *Server) handle(fn func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			s.errorHandler(err)
		}
	}
}

func (s *Server) serviceProviderConfig(w http.ResponseWriter, _ *http.Request) error {
	return protocol.Send(w, http.StatusOK, s.config)
}

func (s *Server) listResourceTypes(w http.ResponseWriter, _ *http.Request) error {
	return protocol.Send(w, http.StatusOK, protocol.NewListResponse(1, len(s.resourceTypes), s.resourceTypes))
}

func (s *Server) resourceTypeByID(w http.ResponseWriter, r *http.Request) error {
	for _, resourceType := range s.resourceTypes {
		if r.PathValue("id") == string(resourceType.ID) {
			return protocol.Send(w, http.StatusOK, resourceType)
		}
	}
	return protocol.SendError(w, scimerrors.ErrNotFound("resource type not found"))
}

func (s *Server) listSchemas(w http.ResponseWriter, _ *http.Request) error {
	return protocol.Send(w, http.StatusOK, protocol.NewListResponse(1, len(s.schemas), s.schemas))
}

func (s *Server) schemaByID(w http.ResponseWriter, r *http.Request) error {
	for _, schema := range s.schemas {
		if r.PathValue("id") == string(schema.ID) {
			return protocol.Send(w, http.StatusOK, schema)
		}
	}
	return protocol.SendError(w, scimerrors.ErrNotFound("schema not found"))
}
