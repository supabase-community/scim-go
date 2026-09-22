package server_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/server"
)

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
		server.NewField[*core.User](core.NewAttribute("emails", core.TypeComplex).AsMultiValued(), nil).With(
			server.NewField(core.NewAttribute("value", core.TypeString), func(u *core.User) any { return u.Emails }),
			server.NewField(core.NewAttribute("type", core.TypeString).Suggesting("work", "home", "other"), func(u *core.User) any { return u.Emails }),
			server.NewField(core.NewAttribute("primary", core.TypeBoolean), func(u *core.User) any { return u.Emails }),
		),
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

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	srv := server.New(basePath).
		WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, userFields()).WithDescription("User Account")).
		WithResource(server.NewResource[*core.Group]("Group", "/Groups", core.SchemaGroup, groupFields())).
		WithErrorHandler(func(err error) { t.Errorf("%v\n", err) }).
		WithAuthentication(core.NewOAuthBearerToken().AsPrimary(), server.RequireBearerToken(
			func(ctx context.Context, candidate string) (context.Context, error) {
				if candidate != validToken {
					return ctx, errors.New("invalid token")
				}
				return ctx, nil
			},
		))

	return Server(t, srv)
}
