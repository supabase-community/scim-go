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
	limits        protocol.Limits
	errorHandler  func(error)
}

type Option func(*Server)

// Limits sets the pagination bounds of every resource type, per RFC 7644, Section 3.4.2.4.
func Limits(limits protocol.Limits) Option {
	return func(s *Server) { s.limits = limits }
}

// ErrorHandler receives the errors the server could not send to a client.
func ErrorHandler(fn func(error)) Option {
	return func(s *Server) { s.errorHandler = fn }
}

func New(basePath string, options ...Option) *Server {
	mux := http.NewServeMux()
	s := &Server{
		mux:          mux,
		handler:      mux,
		basePath:     basePath,
		limits:       protocol.DefaultLimits,
		errorHandler: func(error) {},
	}
	for _, option := range options {
		option(s)
	}
	s.config = core.NewServiceProviderConfig().Sorting().Filtering(s.limits.MaxCount).Patching().Versioning()

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
	resource.mount(s)
	return s
}

// WithAuthentication advertises and enforces scheme, per RFC 7643, Section 5.
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
