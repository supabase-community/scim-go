package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// BearerTokenValidator resolves an RFC 6750 bearer token. It returns the
// context the request should continue with (e.g. carrying a resolved
// tenant for multi-tenant servers) or an error if the token is invalid.
type BearerTokenValidator func(ctx context.Context, token string) (context.Context, error)

// RequireBearerToken enforces RFC 6750 Bearer Token Usage.
func RequireBearerToken(validate BearerTokenValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token, err := bearerToken(r)
			if err != nil {
				challenge(w, "invalid_request", err.Error())
				_ = protocol.SendError(w, scimerrors.ErrUnauthorized(err.Error()))
				return
			}

			ctx, err := validate(r.Context(), token)
			if err != nil {
				challenge(w, "invalid_token", err.Error())
				_ = protocol.SendError(w, scimerrors.ErrUnauthorized(err.Error()))
				return
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// bearerToken extracts the token per RFC 6750, Section 2.1.
func bearerToken(r *http.Request) (string, error) {
	scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
		return "", fmt.Errorf("missing bearer token")
	}
	return token, nil
}

// challenge sets the WWW-Authenticate header per RFC 6750, Section 3.
func challenge(w http.ResponseWriter, errorCode, description string) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer error=%q, error_description=%q`, errorCode, description))
}
