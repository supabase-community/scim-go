package scimtest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/supabase-community/scim-go/pkg/protocol"
)

const (
	requestTimeout = 30 * time.Second
	maxBody        = 16 << 20
)

type Client struct {
	url      string
	http     *http.Client
	requests []func(*http.Request)
}

func NewClient(url string, opts ...Option[*Client]) *Client {
	c := &Client{url: strings.TrimRight(url, "/"), http: &http.Client{Timeout: requestTimeout}}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func Serve(tb testing.TB, h http.Handler, basePath string, opts ...Option[*Client]) *Client {
	tb.Helper()

	srv := httptest.NewServer(h)
	tb.Cleanup(srv.Close)
	return NewClient(srv.URL+basePath, slices.Concat([]Option[*Client]{WithHTTPClient(srv.Client())}, opts)...)
}

func WithHTTPClient(client *http.Client) Option[*Client] {
	return func(c *Client) { c.http = client }
}

func WithRequest(fn func(*http.Request)) Option[*Client] {
	return func(c *Client) { c.requests = append(c.requests, fn) }
}

func WithHeader(key, value string) Option[*Client] {
	return WithRequest(func(r *http.Request) { r.Header.Set(key, value) })
}

func WithBearerToken(token string) Option[*Client] {
	return WithHeader("Authorization", "Bearer "+token)
}

func (c *Client) With(opts ...Option[*Client]) *Client {
	clone := *c
	clone.requests = slices.Clone(c.requests)
	for _, opt := range opts {
		opt(&clone)
	}
	return &clone
}

func (c *Client) Get(tb testing.TB, path string) *http.Response {
	tb.Helper()
	return c.Send(tb, http.MethodGet, path, nil)
}

func (c *Client) Post(tb testing.TB, path string, body any) *http.Response {
	tb.Helper()
	return c.Send(tb, http.MethodPost, path, body)
}

func (c *Client) Put(tb testing.TB, path string, body any) *http.Response {
	tb.Helper()
	return c.Send(tb, http.MethodPut, path, body)
}

func (c *Client) Patch(tb testing.TB, path string, body any) *http.Response {
	tb.Helper()
	return c.Send(tb, http.MethodPatch, path, body)
}

func (c *Client) Delete(tb testing.TB, path string) *http.Response {
	tb.Helper()
	return c.Send(tb, http.MethodDelete, path, nil)
}

func (c *Client) Send(tb testing.TB, method, path string, body any) *http.Response {
	tb.Helper()

	response, err := c.send(tb.Context(), method, path, body)
	if err != nil {
		tb.Fatalf("scimtest: %s %s: %v", method, path, err)
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	_ = response.Body.Close()
	if err == nil && len(data) > maxBody {
		err = fmt.Errorf("response body is larger than %d bytes", maxBody)
	}
	if err != nil {
		tb.Fatalf("scimtest: %s %s: %v", method, path, err)
		return nil
	}
	response.Body = io.NopCloser(bytes.NewReader(data))
	return response
}

func (c *Client) anonymous() *Client {
	transport := http.DefaultTransport
	if shared, ok := c.http.Transport.(*http.Transport); ok {
		transport = shared
	}
	return NewClient(c.url, WithHTTPClient(&http.Client{Transport: transport, Timeout: c.http.Timeout}))
}

func (c *Client) send(ctx context.Context, method, path string, body any) (*http.Response, error) {
	reader, err := encode(body)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, method, c.url+path, reader)
	if err != nil {
		return nil, err
	}
	if reader != nil {
		request.Header.Set("Content-Type", protocol.MediaType)
	}
	for _, fn := range c.requests {
		fn(request)
	}
	return c.http.Do(request)
}

func encode(body any) (io.Reader, error) {
	switch raw := body.(type) {
	case nil:
		return nil, nil
	case []byte:
		if raw == nil {
			return nil, nil
		}
		return bytes.NewReader(raw), nil
	case json.RawMessage:
		return encode([]byte(raw))
	default:
		data, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		return bytes.NewReader(data), nil
	}
}
