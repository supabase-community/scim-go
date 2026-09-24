package server

import (
	"net/http"

	"github.com/supabase-community/scim-go/pkg/core"
)

const DefaultMaxBodySize = 1 << 20

type Option[T any] func(T)

// DefaultCount sets the page size used when a client omits "count", per RFC 7644, Section 3.4.2.4.
func DefaultCount(n int) Option[*Server] {
	return func(s *Server) { s.limits.DefaultCount = n }
}

// ErrorHandler receives the request and the error the server hit while handling it.
func ErrorHandler(fn func(*http.Request, error)) Option[*Server] {
	return func(s *Server) { s.errorHandler = fn }
}

// MaxBodySize caps the request body; larger bodies get 413, per RFC 7644, Section 3.12.
func MaxBodySize(n int64) Option[*Server] {
	return func(s *Server) { s.maxBodySize = n }
}

// WithAuthentication advertises and enforces scheme, per RFC 7643, Section 5.
func WithAuthentication(scheme *core.AuthenticationScheme, middleware func(http.Handler) http.Handler) Option[*Server] {
	return func(s *Server) {
		s.config.Authentication(scheme)
		s.handler = middleware(s.handler)
	}
}

func WithResource(resource Registration) Option[*Server] {
	return func(s *Server) { s.registrations = append(s.registrations, resource) }
}
