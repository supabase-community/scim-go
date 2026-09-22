package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// BearerTokenValidator resolves an RFC 6750 bearer token into a context to continue with, or an error.
type BearerTokenValidator func(ctx context.Context, token string) (context.Context, error)

// RequireBearerToken enforces RFC 6750 Bearer Token Usage.
func RequireBearerToken(validate BearerTokenValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			scheme, token, hasScheme := strings.Cut(r.Header.Get("Authorization"), " ")

			if !hasScheme || !strings.EqualFold(scheme, "Bearer") {
				// RFC 6750 S3.1: no credentials presented -- omit error info.
				w.Header().Set("WWW-Authenticate", "Bearer")
				_ = protocol.SendError(w, scimerrors.ErrUnauthorized("authentication required"))
				return
			}

			if token == "" {
				challenge(w, "invalid_request", "missing bearer token")
				_ = protocol.SendError(w, scimerrors.ErrInvalidSyntax("missing bearer token"))
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

// challenge sets the WWW-Authenticate header per RFC 6750, Section 3.
func challenge(w http.ResponseWriter, errorCode, description string) {
	w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer error=%q, error_description=%q`, errorCode, description))
}
