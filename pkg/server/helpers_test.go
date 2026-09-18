package server_test

import (
	"bytes"
	"context"
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
	return WithRequestHeader("Accept", value)
}

func WithRequestHeader(key, value string) Option[*http.Request] {
	return func(r *http.Request) *http.Request {
		r.Header.Set(key, value)
		return r
	}
}

func WithRequestHeaders(headers map[string]string) Option[*http.Request] {
	return func(r *http.Request) *http.Request {
		for key, value := range headers {
			r = WithRequestHeader(key, value)(r)
		}
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

func WithContext(ctx context.Context) Option[*http.Request] {
	return func(r *http.Request) *http.Request {
		return r.WithContext(ctx)
	}
}
