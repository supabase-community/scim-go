package server_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/server"
)

type tenantKey struct{}

func TestRequireBearerToken(t *testing.T) {
	validate := func(ctx context.Context, token string) (context.Context, error) {
		if token != "good-token" {
			return ctx, errors.New("invalid token")
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
