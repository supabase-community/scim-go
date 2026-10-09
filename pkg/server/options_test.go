package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/server"
)

type failingWriter struct{ *httptest.ResponseRecorder }

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestErrorHandlerOption(t *testing.T) {
	var reported []error
	srv := server.New(basePath, fullServiceProviderConfig(),
		server.ErrorHandler(func(_ *http.Request, err error) { reported = append(reported, err) }),
		server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, core.UserAttributes()...)),
	)

	srv.ServeHTTP(failingWriter{httptest.NewRecorder()}, httptest.NewRequest(http.MethodGet, basePath+"/Users/unknown", nil))

	assert.Len(t, reported, 1)
}

type failingRepository struct{ cause error }

func (r failingRepository) Create(context.Context, *core.User) (*core.User, error) {
	return nil, r.cause
}

func (r failingRepository) Delete(context.Context, *core.User) error { return r.cause }

func (r failingRepository) Read(context.Context, string) (*core.User, error) { return nil, r.cause }

func (r failingRepository) List(context.Context, *protocol.SearchRequest) ([]*core.User, int, error) {
	return nil, 0, r.cause
}

func (r failingRepository) Update(context.Context, *core.User) (*core.User, error) {
	return nil, r.cause
}

func TestErrorHandlerReceivesTheCauseOfAnUnexpectedRepositoryError(t *testing.T) {
	cause := errors.New("pgx: connection pool timeout")
	var reported error
	srv := Server(t, server.New(basePath, fullServiceProviderConfig(),
		server.ErrorHandler(func(_ *http.Request, err error) { reported = err }),
		server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, core.UserAttributes()...).WithRepository(failingRepository{cause: cause})),
	))

	response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users"))

	require.Equal(t, http.StatusInternalServerError, response.StatusCode)
	assert.ErrorIs(t, reported, cause)
}

func TestDefaultCountAfterWithResource(t *testing.T) {
	srv := Server(t, server.New(basePath, fullServiceProviderConfig(),
		server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, core.UserAttributes()...)),
		server.DefaultCount(1),
	))
	for _, name := range []string{"bjensen", "jsmith"} {
		response := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Users", WithRequestBodyAs(t, core.User{UserName: name})))
		require.Equal(t, http.StatusCreated, response.StatusCode)
	}

	response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users"))

	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, 1, ReadBodyAs[protocol.ListResponse[map[string]any]](t, response).ItemsPerPage)
}

func TestMaxBodySize(t *testing.T) {
	srv := Server(t, server.New(basePath, fullServiceProviderConfig(),
		server.MaxBodySize(16),
		server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, core.UserAttributes()...)),
	))

	response := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Users", WithRequestBodyAs(t, core.User{UserName: "bjensen"})))

	assert.Equal(t, http.StatusRequestEntityTooLarge, response.StatusCode)
}

func TestMaxPatchOperations(t *testing.T) {
	patchWith := func(t *testing.T, option server.Option[*server.Server], operations int) int {
		t.Helper()
		var options []server.Option[*server.Server]
		if option != nil {
			options = append(options, option)
		}
		srv, id := newUserWithID(t, core.User{UserName: "bjensen"}, options...)

		request := protocol.PatchRequest{Schemas: []core.SchemaURI{protocol.SchemaPatchOp}}
		for range operations {
			request.Operations = append(request.Operations, patch.Operation{Op: patch.OpReplace, Path: "displayName", Value: json.RawMessage(`"Babs"`)})
		}
		return Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Users/"+id, WithRequestBodyAs(t, request))).StatusCode
	}

	t.Run("accepts up to the default cap", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, patchWith(t, nil, protocol.DefaultLimits.MaxOperations))
	})

	t.Run("refuses more than the default cap with 413", func(t *testing.T) {
		assert.Equal(t, http.StatusRequestEntityTooLarge, patchWith(t, nil, protocol.DefaultLimits.MaxOperations+1))
	})

	t.Run("honours a configured cap", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, patchWith(t, server.MaxPatchOperations(2), 2))
		assert.Equal(t, http.StatusRequestEntityTooLarge, patchWith(t, server.MaxPatchOperations(2), 3))
	})

	t.Run("lifts the cap when set to zero", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, patchWith(t, server.MaxPatchOperations(0), protocol.DefaultLimits.MaxOperations+1))
	})
}

func TestMaxPatchFilterEvaluations(t *testing.T) {
	patchWith := func(t *testing.T, options ...server.Option[*server.Server]) int {
		t.Helper()
		user := core.User{UserName: "bjensen", Emails: []core.Email{{Value: "a@example.com"}, {Value: "b@example.com"}}}
		srv, id := newUserWithID(t, user, options...)

		request := protocol.PatchRequest{
			Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
			Operations: []patch.Operation{{Op: patch.OpReplace, Path: `emails[value eq "b@example.com"].type`, Value: json.RawMessage(`"work"`)}},
		}
		return Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Users/"+id, WithRequestBodyAs(t, request))).StatusCode
	}

	t.Run("accepts a request within the default budget", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, patchWith(t))
	})

	t.Run("refuses a request beyond a configured budget with 413", func(t *testing.T) {
		assert.Equal(t, http.StatusRequestEntityTooLarge, patchWith(t, server.MaxPatchFilterEvaluations(1)))
	})
}

func TestMaxPatchWriteBytes(t *testing.T) {
	patchWith := func(t *testing.T, value string, options ...server.Option[*server.Server]) int {
		t.Helper()
		user := core.User{UserName: "bjensen", Emails: []core.Email{{Value: "a@example.com"}}}
		srv, id := newUserWithID(t, user, options...)

		encoded, err := json.Marshal(value)
		require.NoError(t, err)
		request := protocol.PatchRequest{
			Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
			Operations: []patch.Operation{{Op: patch.OpReplace, Path: `emails[value eq "a@example.com"].value`, Value: encoded}},
		}
		return Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Users/"+id, WithRequestBodyAs(t, request))).StatusCode
	}

	t.Run("accepts a request within the default budget", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, patchWith(t, "b@example.com"))
	})

	t.Run("refuses a request beyond the default budget with 413", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		elements := make([]map[string]any, 2000)
		for i := range elements {
			elements[i] = map[string]any{"type": "a", "n": i}
		}
		add, err := json.Marshal(elements)
		require.NoError(t, err)
		big, err := json.Marshal(strings.Repeat("x", 6000))
		require.NoError(t, err)

		request := Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas: []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{
					{Op: patch.OpAdd, Path: "emails", Value: add},
					{Op: patch.OpReplace, Path: `emails[type eq "a"].value`, Value: big},
				},
			}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusRequestEntityTooLarge, response.StatusCode)
	})

	t.Run("refuses a request beyond a configured budget with 413", func(t *testing.T) {
		assert.Equal(t, http.StatusRequestEntityTooLarge, patchWith(t, strings.Repeat("x", 300), server.MaxPatchWriteBytes(100)))
	})

	t.Run("lifts the cap when set to zero", func(t *testing.T) {
		assert.Equal(t, http.StatusOK, patchWith(t, strings.Repeat("x", 300), server.MaxPatchWriteBytes(0)))
	})
}

func TestListValidatesTheQueryBeforeTheRepository(t *testing.T) {
	srv := Server(t, server.New(basePath, fullServiceProviderConfig(),
		server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, core.UserAttributes()...).WithRepository(failingRepository{cause: errors.New("unreachable")})),
	))

	for query, status := range map[string]int{
		"filter=((garbage":           http.StatusBadRequest,
		"filter=bogus+eq+%22x%22":    http.StatusBadRequest,
		"filter=password+sw+%22x%22": http.StatusForbidden,
		"sortBy=unknown":             http.StatusBadRequest,
	} {
		t.Run(query, func(t *testing.T) {
			response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users?"+query))

			assert.Equal(t, status, response.StatusCode)
		})
	}
}

func TestWithRepository(t *testing.T) {
	attributes := core.UserAttributes()
	repository := server.NewRepository[*core.User](basePath+"/Users", []*core.Schema{core.NewSchema(core.SchemaUser).With(attributes...)})
	existing, err := repository.Create(t.Context(), &core.User{UserName: "bjensen"})
	require.NoError(t, err)

	srv := Server(t, server.New(basePath, fullServiceProviderConfig(), server.WithResource(
		server.NewResource[*core.User]("User", "/Users", core.SchemaUser, attributes...).WithRepository(repository),
	)))

	response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users/"+existing.ID))

	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "bjensen", ReadBodyAs[core.User](t, response).UserName)
}

type tenantKey struct{}

type tenantRepository struct {
	server.Repository[*core.User]
	tenant *string
}

func (r tenantRepository) List(ctx context.Context, query *protocol.SearchRequest) ([]*core.User, int, error) {
	*r.tenant, _ = ctx.Value(tenantKey{}).(string)
	return r.Repository.List(ctx, query)
}

func TestWithAuthentication(t *testing.T) {
	t.Run("reports a validator failure to the ErrorHandler", func(t *testing.T) {
		var reported error
		srv := newTestServer(t, withOption(server.ErrorHandler(func(_ *http.Request, err error) { reported = err })))

		response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users", WithBearerToken(unreachableToken)))

		assert.Equal(t, http.StatusInternalServerError, response.StatusCode)
		assert.ErrorIs(t, reported, errUnreachable)
	})

	t.Run("does not report an invalid token to the ErrorHandler", func(t *testing.T) {
		var reported error
		srv := newTestServer(t, withOption(server.ErrorHandler(func(_ *http.Request, err error) { reported = err })))

		response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users", WithBearerToken("wrong")))

		assert.Equal(t, http.StatusUnauthorized, response.StatusCode)
		assert.NoError(t, reported)
	})

	t.Run("stacks each middleware", func(t *testing.T) {
		var calls []string
		track := func(name string) func(http.Handler) http.Handler {
			return func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls = append(calls, name)
					next.ServeHTTP(w, r)
				})
			}
		}
		srv := newUserServer(t,
			server.WithAuthentication(core.NewOAuthBearerToken().AsPrimary(), track("first")),
			server.WithAuthentication(core.NewOAuthBearerToken(), track("second")),
		)

		response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users"))

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, []string{"second", "first"}, calls)
	})

	t.Run("passes the validator's context to the repository", func(t *testing.T) {
		var tenant string
		schemas := []*core.Schema{core.NewSchema(core.SchemaUser).With(core.UserAttributes()...)}
		repository := tenantRepository{Repository: server.NewRepository[*core.User](basePath+"/Users", schemas), tenant: &tenant}
		srv := Server(t, server.New(basePath, fullServiceProviderConfig(),
			server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, core.UserAttributes()...).WithRepository(repository)),
			server.WithAuthentication(core.NewOAuthBearerToken().AsPrimary(), server.RequireBearerToken(
				func(ctx context.Context, _ string) (context.Context, error) {
					return context.WithValue(ctx, tenantKey{}, "acme"), nil
				},
			)),
		))

		response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users", WithBearerToken("anything")))

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, "acme", tenant)
	})
}

func newUserWithID(t *testing.T, user core.User, options ...server.Option[*server.Server]) (*httptest.Server, string) {
	t.Helper()
	srv := newUserServer(t, options...)
	response := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Users", WithRequestBodyAs(t, user)))
	require.Equal(t, http.StatusCreated, response.StatusCode)
	return srv, ReadBodyAs[core.User](t, response).ID
}

func newUserServer(t *testing.T, options ...server.Option[*server.Server]) *httptest.Server {
	t.Helper()
	options = append(options, server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, core.UserAttributes()...)))
	return Server(t, server.New(basePath, fullServiceProviderConfig(), options...))
}
