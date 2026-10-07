package server_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
	"github.com/supabase-community/scim-go/pkg/server"
)

func TestResourceWithService(t *testing.T) {
	events := &recorder{}
	srv := Server(t, server.New(basePath, fullServiceProviderConfig(),
		server.WithAuthentication(core.NewOAuthBearerToken().AsPrimary(), server.RequireBearerToken(validate)),
		server.WithResource(server.NewResource[*core.Group]("Group", "/Groups", core.SchemaGroup, core.GroupAttributes()...).
			WithService(events.hooks().Wrap)),
	))

	created := createGroup(t, srv, &core.Group{DisplayName: "admins"})

	require.Len(t, events.after, 1)
	assert.Equal(t, server.OpCreate, events.after[0].Op)
	assert.Equal(t, created.ID, events.after[0].ID)
}

func TestResourceWithValidators(t *testing.T) {
	reserved := func(_ context.Context, group *core.Group) error {
		if group.DisplayName == "root" {
			return scimerrors.ErrInvalidValue(`"root" is reserved`)
		}
		return nil
	}
	srv := Server(t, server.New(basePath, fullServiceProviderConfig(),
		server.WithAuthentication(core.NewOAuthBearerToken().AsPrimary(), server.RequireBearerToken(validate)),
		server.WithResource(server.NewResource[*core.Group]("Group", "/Groups", core.SchemaGroup, core.GroupAttributes()...).
			WithValidators(reserved)),
	))

	createGroup(t, srv, &core.Group{DisplayName: "admins"})

	response := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Groups",
		WithBearerToken(validToken),
		WithContentType(protocol.MediaType),
		WithRequestBodyAs(t, &core.Group{DisplayName: "root"}),
	))
	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
	assert.Equal(t, scimerrors.InvalidValue, ReadBodyAs[scimerrors.Error](t, response).ScimType)
}
