package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
	"github.com/supabase-community/scim-go/pkg/server"
)

type widget struct {
	core.Base
	Name  string
	Score int64
	When  time.Time
	Nick  string
	Tags  []any
	Parts []part
}

type part struct {
	Serial string
}

func (w *widget) ResourceID() string { return w.ID }

func widgetAttributes() core.Attributes {
	return core.Attributes{
		core.NewAttribute("name", core.TypeString).AsRequired().UniqueOn(core.UniquenessServer),
		core.NewAttribute("score", core.TypeInteger),
		core.NewAttribute("when", core.TypeDateTime),
		core.NewAttribute("nick", core.TypeString).UniqueOn(core.UniquenessServer).AsCaseExact(),
		core.NewAttribute("tags", core.TypeString).AsMultiValued(),
		core.NewAttribute("parts", core.TypeComplex).AsMultiValued().With(
			core.NewAttribute("serial", core.TypeString).AsRequired(),
		),
	}
}

const widgetSchema core.SchemaURI = "urn:test:widget"

const kitSchema core.SchemaURI = "urn:test:kit"

func kitAttributes() core.Attributes {
	return core.Attributes{
		core.NewAttribute("parts", core.TypeComplex).AsMultiValued().AsRequired().With(
			core.NewAttribute("serial", core.TypeString).AsRequired(),
		),
	}
}

func createWidget(t *testing.T, srv *httptest.Server, w *widget) map[string]any {
	t.Helper()

	request := Request(t, srv, http.MethodPost, basePath+"/Widgets",
		WithBearerToken(validToken),
		WithContentType(protocol.MediaType),
		WithRequestBodyAs(t, w),
	)
	response := Response(t, srv, request)
	require.Equal(t, http.StatusCreated, response.StatusCode)
	return ReadBodyAs[map[string]any](t, response)
}

func create(t *testing.T, srv *httptest.Server, user *core.User) (id, etag string) {
	t.Helper()

	request := Request(t, srv, http.MethodPost, basePath+"/Users",
		WithBearerToken(validToken),
		WithAcceptHeader(protocol.MediaType),
		WithContentType(protocol.MediaType),
		WithRequestBodyAs(t, user),
	)
	response := Response(t, srv, request)
	require.Equal(t, http.StatusCreated, response.StatusCode)

	created := ReadBodyAs[core.User](t, response)
	return created.ID, response.Header.Get("ETag")
}

func patchUser(t *testing.T, srv *httptest.Server, id string, operations ...patch.Operation) core.User {
	t.Helper()

	request := Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
		WithBearerToken(validToken),
		WithContentType(protocol.MediaType),
		WithRequestBodyAs(t, protocol.PatchRequest{
			Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
			Operations: operations,
		}),
	)
	response := Response(t, srv, request)
	require.Equal(t, http.StatusOK, response.StatusCode)
	return ReadBodyAs[core.User](t, response)
}

func userAttributes() core.Attributes {
	return core.Attributes{
		core.NewAttribute("userName", core.TypeString).AsRequired().UniqueOn(core.UniquenessServer),
		core.NewAttribute("name", core.TypeComplex).With(
			core.NewAttribute("givenName", core.TypeString),
			core.NewAttribute("familyName", core.TypeString).AsImmutable(),
		),
		core.NewAttribute("userType", core.TypeString).Suggesting("employee", "contractor"),
		core.NewAttribute("active", core.TypeBoolean),
		core.NewMultiValuedAttribute("emails", "work", "home", "other"),
	}
}

func groupAttributes() core.Attributes {
	return core.Attributes{
		core.NewAttribute("displayName", core.TypeString).AsRequired(),
	}
}

func fullServiceProviderConfig() *core.ServiceProviderConfig {
	return core.NewServiceProviderConfig(basePath).Sorting().Filtering(protocol.DefaultLimits.MaxCount).Patching().Versioning()
}

func standardOptions(t *testing.T) []server.Option[*server.Server] {
	t.Helper()

	return []server.Option[*server.Server]{
		server.ErrorHandler(func(_ *http.Request, err error) { t.Errorf("%v\n", err) }),
		server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, userAttributes()...).WithExtension(core.SchemaEnterpriseUser, enterpriseAttributes()...)),
		server.WithResource(server.NewResource[*core.Group]("Group", "/Groups", core.SchemaGroup, groupAttributes()...)),
		server.WithResource(server.NewResource[*widget]("Widget", "/Widgets", widgetSchema, widgetAttributes()...)),
		server.WithAuthentication(core.NewOAuthBearerToken().AsPrimary(), server.RequireBearerToken(
			func(ctx context.Context, candidate string) (context.Context, error) {
				if candidate != validToken {
					return ctx, server.ErrInvalidToken
				}
				return ctx, nil
			},
		)),
	}
}

func newTestServer(t *testing.T, options ...server.Option[*server.Server]) *httptest.Server {
	t.Helper()

	srv := server.New(fullServiceProviderConfig(), append(options, standardOptions(t)...)...)

	return Server(t, srv)
}

func bearerServer(t *testing.T, validate server.TokenValidator, options ...server.Option[*server.Server]) *httptest.Server {
	t.Helper()

	return Server(t, server.New(fullServiceProviderConfig(), append(options,
		server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, userAttributes()...)),
		server.WithAuthentication(core.NewOAuthBearerToken().AsPrimary(), server.RequireBearerToken(validate)),
	)...))
}

func enterpriseAttributes() core.Attributes {
	return core.Attributes{
		core.NewAttribute("employeeNumber", core.TypeString),
		core.NewAttribute("department", core.TypeString),
	}
}

type racingRepository struct {
	server.Repository[*core.User]
}

func (racingRepository) Replace(context.Context, *core.User) (*core.User, error) {
	return nil, scimerrors.ErrPreconditionFailed("resource has changed on the server")
}
