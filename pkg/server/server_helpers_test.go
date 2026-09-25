package server_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
	"github.com/supabase-community/scim-go/pkg/server"
)

const (
	basePath         = "/scim/v2"
	validToken       = "s3cr3t"
	expiredToken     = "expired"
	unreachableToken = "unreachable"
	racer            = "racer"

	widgetSchema core.SchemaURI = "urn:test:widget"
	kitSchema    core.SchemaURI = "urn:test:kit"
)

var errUnreachable = errors.New("dial tcp 10.0.0.1:5432: connection refused")

var tokens = map[string]error{
	validToken:       nil,
	expiredToken:     fmt.Errorf("%w: expired at noon", server.ErrInvalidToken),
	unreachableToken: errUnreachable,
}

type testServer struct {
	config  *core.ServiceProviderConfig
	options []server.Option[*server.Server]
}

type testOption func(*testServer)

type widget struct {
	core.Base
	Name  string
	Score int64
	When  time.Time
	Nick  string
	Tags  []any
	Parts []part
}

type kit struct {
	core.Base
	Active *bool
	Name   *core.Name
	Parts  []part
}

type part struct {
	Serial string
}

type racingRepository struct {
	server.Repository[*core.User]
}

func newTestServer(t *testing.T, options ...testOption) *httptest.Server {
	t.Helper()

	s := &testServer{config: fullServiceProviderConfig()}
	for _, option := range options {
		option(s)
	}
	users := racingRepository{server.NewRepository[*core.User](basePath+"/Users", core.Schemas{
		core.NewSchema(core.SchemaUser).WithName("User").With(userAttributes()...),
		core.NewSchema(core.SchemaEnterpriseUser).With(enterpriseAttributes()...),
	})}
	standard := []server.Option[*server.Server]{
		server.ErrorHandler(func(_ *http.Request, err error) {
			if !errors.Is(err, errUnreachable) {
				t.Errorf("%v\n", err)
			}
		}),
		server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, userAttributes()...).
			WithExtension(core.SchemaEnterpriseUser, enterpriseAttributes()...).
			WithRepository(users)),
		server.WithResource(server.NewResource[*core.Group]("Group", "/Groups", core.SchemaGroup, core.GroupAttributes()...)),
		server.WithResource(server.NewResource[*widget]("Widget", "/Widgets", widgetSchema, widgetAttributes()...)),
		server.WithResource(server.NewResource[*kit]("Kit", "/Kits", kitSchema, kitAttributes()...)),
		server.WithAuthentication(core.NewOAuthBearerToken().AsPrimary(), server.RequireBearerToken(validate)),
	}
	return Server(t, server.New(s.config, slices.Concat(standard, s.options)...))
}

func withConfig(config *core.ServiceProviderConfig) testOption {
	return func(s *testServer) { s.config = config }
}

func withOption(options ...server.Option[*server.Server]) testOption {
	return func(s *testServer) { s.options = append(s.options, options...) }
}

func (r racingRepository) Replace(ctx context.Context, user *core.User) (*core.User, error) {
	if user.UserName == racer {
		return nil, scimerrors.ErrPreconditionFailed("resource has changed on the server")
	}
	return r.Repository.Replace(ctx, user)
}

func validate(ctx context.Context, token string) (context.Context, error) {
	err, ok := tokens[token]
	if !ok {
		return ctx, server.ErrInvalidToken
	}
	return ctx, err
}

func fullServiceProviderConfig() *core.ServiceProviderConfig {
	return core.NewServiceProviderConfig(basePath).Sorting().Filtering(protocol.DefaultLimits.MaxCount).Patching().Versioning()
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

func enterpriseAttributes() core.Attributes {
	return core.Attributes{
		core.NewAttribute("employeeNumber", core.TypeString).AsImmutable(),
		core.NewAttribute("department", core.TypeString),
	}
}

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

func kitAttributes() core.Attributes {
	return core.Attributes{
		core.NewAttribute("active", core.TypeBoolean).AsRequired(),
		core.NewAttribute("name", core.TypeComplex).AsRequired().With(core.NewAttribute("givenName", core.TypeString)),
		core.NewAttribute("parts", core.TypeComplex).AsMultiValued().AsRequired().With(
			core.NewAttribute("serial", core.TypeString).AsRequired(),
		),
	}
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
