package server_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimtest"
)

var authorized = scimtest.WithBearerToken(validToken)

func ConcurrentResponses(t *testing.T, srv *httptest.Server, requests ...*http.Request) []*http.Response {
	t.Helper()

	responses := make([]*http.Response, len(requests))
	errs := make([]error, len(requests))
	var wg sync.WaitGroup
	for i, request := range requests {
		wg.Go(func() { responses[i], errs[i] = srv.Client().Do(request) })
	}
	wg.Wait()
	for _, response := range responses {
		if response != nil {
			t.Cleanup(func() { _ = response.Body.Close() })
		}
	}
	require.NoError(t, errors.Join(errs...))
	return responses
}

func rawRequest(t *testing.T, srv *httptest.Server, method, path string, body any) *http.Request {
	t.Helper()

	var reader io.Reader
	if body != nil {
		raw, ok := body.([]byte)
		if !ok {
			var err error
			raw, err = json.Marshal(body)
			require.NoError(t, err)
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequestWithContext(t.Context(), method, srv.URL+path, reader)
	require.NoError(t, err)
	request.Header.Set("Authorization", "Bearer "+validToken)
	request.Header.Set("Content-Type", protocol.MediaType)
	return request
}

func ifMatch(request *http.Request, etag string) *http.Request {
	request.Header.Set("If-Match", etag)
	return request
}

func rawServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv
}

func newClient(t *testing.T, handler http.Handler, opts ...scimtest.Option[*scimtest.Client]) *scimtest.Client {
	t.Helper()
	return scimtest.Serve(t, handler, "", opts...)
}

func clientOf(srv *httptest.Server) *scimtest.Client {
	return scimtest.NewClient(srv.URL, scimtest.WithHTTPClient(srv.Client()), scimtest.WithBearerToken(validToken))
}

func statusCodes(responses []*http.Response) []int {
	codes := make([]int, len(responses))
	for i, response := range responses {
		codes[i] = response.StatusCode
	}
	return codes
}

func as[T any](t *testing.T, v any) T {
	t.Helper()
	got, ok := v.(T)
	require.True(t, ok, "%T is not the expected type", v)
	return got
}
