package server_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/server"
)

type tenantKey struct{}

func TestRequireBearerToken(t *testing.T) {
	validate := func(ctx context.Context, token string) (context.Context, error) {
		switch token {
		case "good-token":
		case "expired-token":
			return ctx, fmt.Errorf("%w: expired at noon", server.ErrInvalidToken)
		case "db-down":
			return ctx, errors.New("dial tcp 10.0.0.1:5432: connection refused")
		default:
			return ctx, server.ErrInvalidToken
		}
		return context.WithValue(ctx, tenantKey{}, "acme"), nil
	}

	newHandler := func(called *bool, tenant *string) http.Handler {
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			*called = true
			if v, ok := r.Context().Value(tenantKey{}).(string); ok {
				*tenant = v
			}
			w.WriteHeader(http.StatusOK)
		})
		return server.RequireBearerToken(validate)(next)
	}

	t.Run("rejects a request with no Authorization header, omitting error info per RFC 6750 S3.1", func(t *testing.T) {
		var called bool
		var tenant string
		handler := newHandler(&called, &tenant)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		handler.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.False(t, called)
		assert.Equal(t, "Bearer", w.Header().Get("WWW-Authenticate"))
	})

	t.Run("rejects a request with a non-Bearer scheme, omitting error info per RFC 6750 S3.1", func(t *testing.T) {
		var called bool
		var tenant string
		handler := newHandler(&called, &tenant)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
		handler.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.False(t, called)
		assert.Equal(t, "Bearer", w.Header().Get("WWW-Authenticate"))
	})

	t.Run("rejects a Bearer scheme with an empty token as a malformed request", func(t *testing.T) {
		var called bool
		var tenant string
		handler := newHandler(&called, &tenant)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer ")
		handler.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.False(t, called)
		assert.Contains(t, w.Header().Get("WWW-Authenticate"), `error="invalid_request"`)
	})

	t.Run("rejects an invalid token", func(t *testing.T) {
		var called bool
		var tenant string
		handler := newHandler(&called, &tenant)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer bad-token")
		handler.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.False(t, called)
		assert.Contains(t, w.Header().Get("WWW-Authenticate"), `error="invalid_token"`)
	})

	t.Run("rejects a wrapped invalid token with a fixed description", func(t *testing.T) {
		var called bool
		var tenant string
		handler := newHandler(&called, &tenant)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer expired-token")
		handler.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.False(t, called)
		assert.Equal(t, `Bearer error="invalid_token", error_description="The access token is invalid"`, w.Header().Get("WWW-Authenticate"))
		assert.NotContains(t, w.Body.String(), "noon")
	})

	t.Run("answers a validator failure with 500 and no challenge", func(t *testing.T) {
		var called bool
		var tenant string
		handler := newHandler(&called, &tenant)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer db-down")
		handler.ServeHTTP(w, r)

		body, _ := io.ReadAll(w.Body)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.False(t, called)
		assert.Empty(t, w.Header().Get("WWW-Authenticate"))
		assert.NotContains(t, string(body), "10.0.0.1")
	})

	t.Run("passes a validator failure to the ErrorHandler", func(t *testing.T) {
		cause := errors.New("connection refused")
		var reported error
		srv := server.New(fullServiceProviderConfig(),
			server.ErrorHandler(func(_ *http.Request, err error) { reported = err }),
			server.WithAuthentication(core.NewOAuthBearerToken().AsPrimary(), server.RequireBearerToken(
				func(ctx context.Context, _ string) (context.Context, error) { return ctx, cause },
			)),
		)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, basePath+"/ServiceProviderConfig", nil)
		r.Header.Set("Authorization", "Bearer anything")
		srv.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.ErrorIs(t, reported, cause)
	})

	t.Run("does not report an invalid token to the ErrorHandler", func(t *testing.T) {
		var reported error
		srv := server.New(fullServiceProviderConfig(),
			server.ErrorHandler(func(_ *http.Request, err error) { reported = err }),
			server.WithAuthentication(core.NewOAuthBearerToken().AsPrimary(), server.RequireBearerToken(
				func(ctx context.Context, _ string) (context.Context, error) { return ctx, server.ErrInvalidToken },
			)),
		)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, basePath+"/ServiceProviderConfig", nil)
		r.Header.Set("Authorization", "Bearer anything")
		srv.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.NoError(t, reported)
	})

	t.Run("accepts a valid token and threads the validator's context to the next handler", func(t *testing.T) {
		var called bool
		var tenant string
		handler := newHandler(&called, &tenant)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "Bearer good-token")
		handler.ServeHTTP(w, r)

		require.True(t, called)
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "acme", tenant)
	})

	t.Run("matches the Bearer scheme case-insensitively", func(t *testing.T) {
		var called bool
		var tenant string
		handler := newHandler(&called, &tenant)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Authorization", "bearer good-token")
		handler.ServeHTTP(w, r)

		assert.True(t, called)
		assert.Equal(t, http.StatusOK, w.Code)
	})
}
