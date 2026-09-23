package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

type Option[T any] func(T) T

func Request(t *testing.T, srv *httptest.Server, method, path string, options ...Option[*http.Request]) *http.Request {
	t.Helper()

	request, err := http.NewRequest(method, srv.URL+path, nil)
	require.NoError(t, err)
	for _, option := range options {
		request = option(request)
	}
	return request
}

func Response(t *testing.T, srv *httptest.Server, r *http.Request) *http.Response {
	t.Helper()

	response, err := srv.Client().Do(r)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = response.Body.Close()
	})
	return response
}

func Server(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func WithAcceptHeader(value string) Option[*http.Request] {
	return WithHeader("Accept", value)
}

func WithContentType(value string) Option[*http.Request] {
	return WithHeader("Content-Type", value)
}

func WithBearerToken(token string) Option[*http.Request] {
	return WithHeader("Authorization", "Bearer "+token)
}

func WithHeader(key, value string) Option[*http.Request] {
	return func(r *http.Request) *http.Request {
		r.Header.Set(key, value)
		return r
	}
}

func WithRequestBody(raw []byte) Option[*http.Request] {
	return func(r *http.Request) *http.Request {
		if raw != nil {
			r.Body = io.NopCloser(bytes.NewReader(raw))
		}
		return r
	}
}

func WithRequestBodyAs[T any](t *testing.T, item T) Option[*http.Request] {
	body, err := json.Marshal(item)
	require.NoError(t, err)
	return WithRequestBody(body)
}

func WithContext(ctx context.Context) Option[*http.Request] {
	return func(r *http.Request) *http.Request {
		return r.WithContext(ctx)
	}
}

func ReadBodyAs[T any](t *testing.T, w *http.Response) T {
	t.Helper()

	body, err := io.ReadAll(w.Body)
	require.NoError(t, err)

	var item T
	require.NoError(t, json.Unmarshal(body, &item))
	return item
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
