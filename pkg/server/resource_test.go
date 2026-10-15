package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
	"github.com/supabase-community/scim-go/pkg/scimtest"
	"github.com/supabase-community/scim-go/pkg/server"
)

func TestResourceWithService(t *testing.T) {
	events := &recorder{}
	srv := newClient(t, server.New(basePath, fullServiceProviderConfig(),
		server.WithAuthentication(core.NewOAuthBearerToken().AsPrimary(), server.RequireBearerToken(validate)),
		server.WithResource(server.NewResource[*core.Group]("Group", "/Groups", core.SchemaGroup, core.GroupAttributes()...).
			WithService(events.hooks().Wrap)),
	), authorized)

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
	srv := newClient(t, server.New(basePath, fullServiceProviderConfig(),
		server.WithAuthentication(core.NewOAuthBearerToken().AsPrimary(), server.RequireBearerToken(validate)),
		server.WithResource(server.NewResource[*core.Group]("Group", "/Groups", core.SchemaGroup, core.GroupAttributes()...).
			WithValidators(reserved)),
	), authorized)

	createGroup(t, srv, &core.Group{DisplayName: "admins"})

	response := srv.Post(t, basePath+"/Groups", &core.Group{DisplayName: "root"})
	assert.Equal(t, http.StatusBadRequest, response.StatusCode)
	assert.Equal(t, scimerrors.InvalidValue, scimtest.Decode[scimerrors.Error](t, response).ScimType)
}

func TestResourceValidatorsSeeTheFullProjection(t *testing.T) {
	var returned []bool
	record := func(ctx context.Context, _ *core.Group) error {
		returned = append(returned, protocol.ProjectionFrom(ctx).Returns("members"))
		return nil
	}
	srv := newClient(t, server.New(basePath, fullServiceProviderConfig(),
		server.WithAuthentication(core.NewOAuthBearerToken().AsPrimary(), server.RequireBearerToken(validate)),
		server.WithDatabase(server.NewDatabase()),
		server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, core.UserAttributes()...)),
		server.WithResource(server.NewResource[*core.Group]("Group", "/Groups", core.SchemaGroup, core.GroupAttributes()...).
			WithValidators(record)),
	), authorized)
	ids := seedUsers(t, srv, "u-1")
	created := createGroup(t, srv, &core.Group{DisplayName: "eng"})

	response := srv.Patch(t, basePath+"/Groups/"+created.ID, protocol.PatchRequest{
		Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
		Operations: []patch.Operation{{Op: patch.OpAdd, Path: "members", Value: json.RawMessage(ids.fill(`[{"value":"u-1","type":"User"}]`))}},
	})

	require.Equal(t, http.StatusNoContent, response.StatusCode)
	assert.Equal(t, []bool{true, true}, returned)
}
