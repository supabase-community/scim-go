package server

import (
	"net/http"

	"github.com/supabase-community/scim-go/pkg/core"
)

type Option[T any] func(T)

func ErrorHandler(fn func(*http.Request, error)) Option[*Server] {
	return func(s *Server) { s.errorHandler = fn }
}

// DefaultCount sets the page size used when a client omits "count", per RFC 7644, Section 3.4.2.4.
func DefaultCount(n int) Option[*Server] {
	return func(s *Server) { s.limits.DefaultCount = n }
}

// MaxPatchOperations caps the operations of one PATCH request; larger requests get 413, and zero lifts the cap.
func MaxPatchOperations(n int) Option[*Server] {
	return func(s *Server) { s.limits.MaxOperations = n }
}

// MaxPatchFilterEvaluations caps the value filter clause checks of one PATCH request; costlier requests get 413, and zero lifts the cap.
func MaxPatchFilterEvaluations(n int) Option[*Server] {
	return func(s *Server) { s.limits.MaxFilterEvaluations = n }
}

// MaxPatchWriteBytes caps the bytes a PATCH request's add and replace operations may write; costlier requests get 413, and zero lifts the cap.
func MaxPatchWriteBytes(n int) Option[*Server] {
	return func(s *Server) { s.limits.MaxWriteBytes = n }
}

// MaxPatchRetries sets how many times a PATCH without If-Match that loses a race to another write is reapplied before it gets 409; zero disables the retry.
func MaxPatchRetries(n int) Option[*Server] {
	return func(s *Server) { s.limits.PatchRetries = n }
}

// MaxBodySize caps the request body; larger bodies get 413, per RFC 7644, Section 3.12.
func MaxBodySize(n int64) Option[*Server] {
	return func(s *Server) { s.maxBodySize = n }
}

// MaxResourceBytes caps the encoded size a resource may reach after a write; larger results get 413, and zero lifts the cap.
func MaxResourceBytes(n int) Option[*Server] {
	return func(s *Server) { s.limits.MaxResourceBytes = n }
}

// WithBaseURL sets the public base URL that each generated meta.location starts with, per RFC 7643, Section 3.1.
func WithBaseURL(baseURL string) Option[*Server] {
	return func(s *Server) { s.baseURL = baseURL }
}

func WithResource(resource Registration) Option[*Server] {
	return func(s *Server) { s.registrations = append(s.registrations, resource) }
}

// WithAuthentication advertises scheme and enforces it on every endpoint but /ServiceProviderConfig, per RFC 7643, Section 5.
func WithAuthentication(scheme *core.AuthenticationScheme, middleware func(http.Handler) http.Handler) Option[*Server] {
	return func(s *Server) {
		s.config.Authentication(scheme)
		previous := s.authenticate
		s.authenticate = func(next http.Handler) http.Handler { return middleware(previous(next)) }
	}
}
