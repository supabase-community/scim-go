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
	maxBodySize   int64
	registrations []Registration
	errorHandler  func(*http.Request, error)
}

const DefaultMaxBodySize = 1 << 20

type Option[T any] func(T)

// ErrorHandler receives the request and the error the server hit while handling it.
func ErrorHandler(fn func(*http.Request, error)) Option[*Server] {
	return func(s *Server) { s.errorHandler = fn }
}

// DefaultCount sets the page size used when a client omits "count", per RFC 7644, Section 3.4.2.4.
func DefaultCount(n int) Option[*Server] {
	return func(s *Server) { s.limits.DefaultCount = n }
}

// MaxBodySize caps the request body; larger bodies get 413, per RFC 7644, Section 3.12.
func MaxBodySize(n int64) Option[*Server] {
	return func(s *Server) { s.maxBodySize = n }
}

func WithResource(resource Registration) Option[*Server] {
	return func(s *Server) { s.registrations = append(s.registrations, resource) }
}

// WithAuthentication advertises and enforces scheme, per RFC 7643, Section 5.
func WithAuthentication(scheme *core.AuthenticationScheme, middleware func(http.Handler) http.Handler) Option[*Server] {
	return func(s *Server) {
		s.config.Authentication(scheme)
		s.handler = middleware(s.handler)
	}
}

func New(config *core.ServiceProviderConfig, options ...Option[*Server]) *Server {
	mux := http.NewServeMux()
	s := &Server{
		mux:          mux,
		basePath:     config.BasePath(),
		config:       config,
		limits:       limitsFrom(config),
		maxBodySize:  DefaultMaxBodySize,
		errorHandler: func(*http.Request, error) {},
	}
	s.handler = http.HandlerFunc(s.route)
	for _, option := range options {
		option(s)
	}
	for _, resource := range s.registrations {
		s.mount(resource)
	}

	mux.HandleFunc("GET "+s.basePath+"/ServiceProviderConfig", s.handle(s.serviceProviderConfig))
	mux.HandleFunc("GET "+s.basePath+"/ResourceTypes", s.handle(s.listResourceTypes))
	mux.HandleFunc("GET "+s.basePath+"/ResourceTypes/{id}", s.handle(s.resourceTypeByID))
	mux.HandleFunc("GET "+s.basePath+"/Schemas", s.handle(s.listSchemas))
	mux.HandleFunc("GET "+s.basePath+"/Schemas/{id}", s.handle(s.schemaByID))
	mux.HandleFunc(s.basePath+"/Me", s.handle(me))

	return s
}

// limitsFrom derives the page-size cap from the advertised filter.maxResults, per RFC 7643, Section 5.
func limitsFrom(config *core.ServiceProviderConfig) protocol.Limits {
	limits := protocol.DefaultLimits
	if config.Filter.MaxResults > 0 {
		limits.MaxCount = config.Filter.MaxResults
	}
	return limits
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	r = withReporter(r, s.errorHandler)
	r.Body = http.MaxBytesReader(w, r.Body, s.maxBodySize)
	s.handler.ServeHTTP(w, r)
}

func (s *Server) handle(fn func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			s.errorHandler(r, err)
		}
	}
}

func (s *Server) listResourceTypes(w http.ResponseWriter, r *http.Request) error {
	if err := rejectFilter(r); err != nil {
		return protocol.SendError(w, err)
	}
	return protocol.Send(w, http.StatusOK, protocol.NewListResponse(1, len(s.resourceTypes), s.resourceTypes))
}

type unmatched struct {
	header http.Header
	status int
}

func (u *unmatched) Header() http.Header { return u.header }

func (u *unmatched) Write(b []byte) (int, error) { return len(b), nil }

func (u *unmatched) WriteHeader(status int) { u.status = status }

// me declines the "/Me" alias, per RFC 7644, Section 3.11.
func me(w http.ResponseWriter, _ *http.Request) error {
	return protocol.SendError(w, scimerrors.ErrNotImplemented(`"/Me" is not supported`))
}

func (s *Server) listSchemas(w http.ResponseWriter, r *http.Request) error {
	if err := rejectFilter(r); err != nil {
		return protocol.SendError(w, err)
	}
	return protocol.Send(w, http.StatusOK, protocol.NewListResponse(1, len(s.schemas), s.schemas))
}

func (s *Server) mount(resource Registration) {
	schemas := resource.schemas(s.basePath)
	s.resourceTypes = append(s.resourceTypes, resource.resourceType(s.basePath))
	s.schemas = append(s.schemas, schemas...)
	resource.mount(s, schemas)
}

func (s *Server) resourceTypeByID(w http.ResponseWriter, r *http.Request) error {
	for _, resourceType := range s.resourceTypes {
		if r.PathValue("id") == string(resourceType.ID) {
			return protocol.Send(w, http.StatusOK, resourceType)
		}
	}
	return protocol.SendError(w, scimerrors.ErrNotFound("resource type not found"))
}

// route answers a request no endpoint matches with a SCIM error, per RFC 7644, Section 3.12.
func (s *Server) route(w http.ResponseWriter, r *http.Request) {
	if _, pattern := s.mux.Handler(r); pattern != "" {
		s.mux.ServeHTTP(w, r)
		return
	}
	unmatched := &unmatched{header: http.Header{}}
	s.mux.ServeHTTP(unmatched, r)
	if allow := unmatched.header.Get("Allow"); allow != "" {
		w.Header().Set("Allow", allow)
	}
	report(r, protocol.SendError(w, scimerrors.NewError(unmatched.status, "", http.StatusText(unmatched.status))))
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

// rejectFilter forbids "filter" on /ResourceTypes and /Schemas, per RFC 7644, Section 4.
func rejectFilter(r *http.Request) error {
	if r.URL.Query().Get("filter") != "" {
		return scimerrors.ErrForbidden(`"filter" is not supported on this endpoint`)
	}
	return nil
}
