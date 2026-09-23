package server_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
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

func widgetFields() server.Fields[*widget] {
	return server.NewFields(
		server.NewField(
			core.NewAttribute("name", core.TypeString).AsRequired().UniqueOn(core.UniquenessServer),
			func(w *widget) any { return w.Name },
		),
		server.NewField(
			core.NewAttribute("score", core.TypeInteger),
			func(w *widget) any { return w.Score },
		),
		server.NewField(
			core.NewAttribute("when", core.TypeDateTime),
			func(w *widget) any { return w.When },
		),
		server.NewField(
			core.NewAttribute("nick", core.TypeString).UniqueOn(core.UniquenessServer).AsCaseExact(),
			func(w *widget) any {
				if w.Nick == "" {
					return nil
				}
				return w.Nick
			},
		),
		server.NewField(
			core.NewAttribute("tags", core.TypeString).AsMultiValued(),
			func(w *widget) any { return w.Tags },
		),
		server.NewElements(
			core.NewAttribute("parts", core.TypeComplex).AsMultiValued(),
			func(w *widget) []part { return w.Parts },
			server.NewField(core.NewAttribute("serial", core.TypeString).AsRequired(), func(p part) any { return p.Serial }),
		),
	)
}

const widgetSchema core.SchemaURI = "urn:test:widget"

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

func userFields() server.Fields[*core.User] {
	return server.NewFields(
		server.NewField(
			core.NewAttribute("userName", core.TypeString).AsRequired().UniqueOn(core.UniquenessServer),
			func(u *core.User) any { return u.UserName },
		),
		server.NewField[*core.User](core.NewAttribute("name", core.TypeComplex), nil).With(
			server.NewField(core.NewAttribute("givenName", core.TypeString), func(u *core.User) any { return u.Name.GivenName }),
			server.NewField(core.NewAttribute("familyName", core.TypeString).AsImmutable(), func(u *core.User) any { return u.Name.FamilyName }),
		),
		server.NewField(
			core.NewAttribute("userType", core.TypeString).Suggesting("employee", "contractor"),
			func(u *core.User) any { return u.UserType },
		),
		server.NewField(
			core.NewAttribute("active", core.TypeBoolean),
			func(u *core.User) any {
				if u.Active == nil {
					return nil
				}
				return *u.Active
			},
		),
		server.NewMultiValued("emails", func(u *core.User) []core.Email { return u.Emails }, "work", "home", "other"),
	)
}

func groupFields() server.Fields[*core.Group] {
	return server.NewFields(
		server.NewField(
			core.NewAttribute("displayName", core.TypeString).AsRequired(),
			func(g *core.Group) any { return g.DisplayName },
		),
	)
}

func fullServiceProviderConfig() *core.ServiceProviderConfig {
	return core.NewServiceProviderConfig(basePath).Sorting().Filtering(protocol.DefaultLimits.MaxCount).Patching().Versioning()
}

func standardOptions(t *testing.T) []server.Option[*server.Server] {
	t.Helper()

	return []server.Option[*server.Server]{
		server.ErrorHandler(func(err error) { t.Errorf("%v\n", err) }),
		server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, userFields()).WithExtension(core.SchemaEnterpriseUser, enterpriseFields())),
		server.WithResource(server.NewResource[*core.Group]("Group", "/Groups", core.SchemaGroup, groupFields())),
		server.WithResource(server.NewResource[*widget]("Widget", "/Widgets", widgetSchema, widgetFields())),
		server.WithAuthentication(core.NewOAuthBearerToken().AsPrimary(), server.RequireBearerToken(
			func(ctx context.Context, candidate string) (context.Context, error) {
				if candidate != validToken {
					return ctx, errors.New("invalid token")
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

func enterpriseFields() server.Fields[*core.User] {
	enterprise := func(read func(*core.EnterpriseUser) any) server.Accessor[*core.User] {
		return func(u *core.User) any {
			if u.EnterpriseUser == nil {
				return nil
			}
			return read(u.EnterpriseUser)
		}
	}
	return server.NewFields(
		server.NewField(core.NewAttribute("employeeNumber", core.TypeString), enterprise(func(e *core.EnterpriseUser) any { return e.EmployeeNumber })),
		server.NewField(core.NewAttribute("department", core.TypeString), enterprise(func(e *core.EnterpriseUser) any { return e.Department })),
	)
}
