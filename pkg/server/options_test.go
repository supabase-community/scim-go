package server_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/server"
)

type failingWriter struct{ *httptest.ResponseRecorder }

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestErrorHandlerOption(t *testing.T) {
	var reported []error
	srv := server.New(fullServiceProviderConfig(),
		server.ErrorHandler(func(_ *http.Request, err error) { reported = append(reported, err) }),
		server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, userAttributes()...)),
	)

	srv.ServeHTTP(failingWriter{httptest.NewRecorder()}, httptest.NewRequest(http.MethodGet, basePath+"/Users/unknown", nil))

	assert.Len(t, reported, 1)
}

type failingRepository struct{ cause error }

func (r failingRepository) Create(context.Context, *core.User) (*core.User, error) {
	return nil, r.cause
}

func (r failingRepository) Delete(context.Context, string, string) error { return r.cause }

func (r failingRepository) Get(context.Context, string) (*core.User, error) { return nil, r.cause }

func (r failingRepository) List(context.Context, *protocol.SearchRequest) ([]*core.User, int, error) {
	return nil, 0, r.cause
}

func (r failingRepository) Replace(context.Context, *core.User) (*core.User, error) {
	return nil, r.cause
}

func TestErrorHandlerReceivesTheCauseOfAnUnexpectedRepositoryError(t *testing.T) {
	cause := errors.New("pgx: connection pool timeout")
	var reported error
	srv := Server(t, server.New(fullServiceProviderConfig(),
		server.ErrorHandler(func(_ *http.Request, err error) { reported = err }),
		server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, userAttributes()...).WithRepository(failingRepository{cause: cause})),
	))

	response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users"))

	require.Equal(t, http.StatusInternalServerError, response.StatusCode)
	assert.ErrorIs(t, reported, cause)
}

func TestDefaultCountAfterWithResource(t *testing.T) {
	srv := Server(t, server.New(fullServiceProviderConfig(),
		server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, userAttributes()...)),
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
	srv := Server(t, server.New(fullServiceProviderConfig(),
		server.MaxBodySize(16),
		server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, userAttributes()...)),
	))

	response := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Users", WithRequestBodyAs(t, core.User{UserName: "bjensen"})))

	assert.Equal(t, http.StatusRequestEntityTooLarge, response.StatusCode)
}

func TestListValidatesTheQueryBeforeTheRepository(t *testing.T) {
	srv := Server(t, server.New(fullServiceProviderConfig(),
		server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, userAttributes()...).WithRepository(failingRepository{cause: errors.New("unreachable")})),
	))

	for _, query := range []string{"filter=((garbage", "sortBy=unknown"} {
		t.Run(query, func(t *testing.T) {
			response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users?"+query))

			assert.Equal(t, http.StatusBadRequest, response.StatusCode)
		})
	}
}

func TestWithRepository(t *testing.T) {
	attributes := userAttributes()
	repository := server.NewRepository[*core.User](basePath+"/Users", []*core.Schema{core.NewSchema(core.SchemaUser).With(attributes...)})
	existing, err := repository.Create(t.Context(), &core.User{UserName: "bjensen"})
	require.NoError(t, err)

	srv := Server(t, server.New(fullServiceProviderConfig(), server.WithResource(
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

	t.Run("passes the validator's context to the repository", func(t *testing.T) {
		var tenant string
		schemas := []*core.Schema{core.NewSchema(core.SchemaUser).With(userAttributes()...)}
		repository := tenantRepository{Repository: server.NewRepository[*core.User](basePath+"/Users", schemas), tenant: &tenant}
		srv := Server(t, server.New(fullServiceProviderConfig(),
			server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, userAttributes()...).WithRepository(repository)),
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
