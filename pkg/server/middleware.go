package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// ErrInvalidToken rejects a bearer token, per RFC 6750, Section 3.1.
var ErrInvalidToken = errors.New("invalid token")

const invalidTokenDescription = "The access token is invalid"

// TokenValidator resolves an RFC 6750 bearer token into a context to continue with, or an error.
type TokenValidator func(ctx context.Context, token string) (context.Context, error)

// RequireBearerToken enforces RFC 6750 Bearer Token Usage.
func RequireBearerToken(validate TokenValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scheme, token, hasScheme := strings.Cut(r.Header.Get("Authorization"), " ")

			if !hasScheme || !strings.EqualFold(scheme, "Bearer") {
				w.Header().Set("WWW-Authenticate", "Bearer")
				_ = protocol.SendError(w, scimerrors.ErrUnauthorized("authentication required"))
				return
			}

			ctx, err := validate(r.Context(), token)
			if errors.Is(err, ErrInvalidToken) {
				challenge(w, "invalid_token", invalidTokenDescription)
				_ = protocol.SendError(w, scimerrors.ErrUnauthorized(invalidTokenDescription))
				return
			}
			if err != nil {
				report(r, protocol.SendError(w, err))
				return
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// challenge sets the WWW-Authenticate header per RFC 6750, Section 3.
func challenge(w http.ResponseWriter, errorCode, description string) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer error=%q, error_description=%q`, errorCode, description))
}

type reporterKey struct{}

func withReporter(r *http.Request, fn func(*http.Request, error)) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), reporterKey{}, fn))
}

func report(r *http.Request, err error) {
	if fn, ok := r.Context().Value(reporterKey{}).(func(*http.Request, error)); ok && err != nil {
		fn(r, err)
	}
}
