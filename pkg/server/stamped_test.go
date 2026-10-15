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
	"github.com/supabase-community/scim-go/pkg/scimtest"
	"github.com/supabase-community/scim-go/pkg/server"
)

type bareRepository struct {
	server.Repository[*core.User]
}

func (r bareRepository) Create(ctx context.Context, user *core.User) (*core.User, error) {
	user, err := r.Repository.Create(ctx, user)
	return strip(user), err
}

func (r bareRepository) Read(ctx context.Context, id string) (*core.User, error) {
	user, err := r.Repository.Read(ctx, id)
	return strip(user), err
}

func (r bareRepository) Update(ctx context.Context, user *core.User) (*core.User, error) {
	user, err := r.Repository.Update(ctx, user)
	return strip(user), err
}

func (r bareRepository) List(ctx context.Context, query *protocol.SearchRequest) ([]*core.User, int, error) {
	users, total, err := r.Repository.List(ctx, query)
	for _, user := range users {
		strip(user)
	}
	return users, total, err
}

func TestServerStampsMetaOnWhatTheRepositoryReturns(t *testing.T) {
	schemas := core.Schemas{core.NewSchema(core.SchemaUser).With(core.UserAttributes()...)}
	client := newClient(t, server.New(basePath, fullServiceProviderConfig(), server.WithBaseURL("https://example.com/scim/v2"), server.WithResource(
		server.NewResource[*core.User]("User", "/Users", core.SchemaUser, core.UserAttributes()...).
			WithRepository(bareRepository{server.NewRepository[*core.User]("", schemas)}),
	)))
	created := scimtest.Decode[core.User](t, client.Post(t, basePath+"/Users", core.User{UserName: "bjensen"}))
	patch := protocol.PatchRequest{Schemas: []core.SchemaURI{protocol.SchemaPatchOp}, Operations: []patch.Operation{{Op: patch.OpReplace, Path: "displayName", Value: json.RawMessage(`"Barbara"`)}}}

	for name, response := range map[string]*http.Response{
		"create":  client.Post(t, basePath+"/Users", core.User{UserName: "ajensen"}),
		"get":     client.Get(t, basePath+"/Users/"+created.ID),
		"replace": client.Put(t, basePath+"/Users/"+created.ID, core.User{UserName: "bjensen", DisplayName: "Babs"}),
		"patch":   client.Patch(t, basePath+"/Users/"+created.ID, patch),
	} {
		t.Run(name, func(t *testing.T) {
			require.Less(t, response.StatusCode, 300)
			user := scimtest.Decode[core.User](t, response)
			assert.Equal(t, core.ResourceTypeName("User"), user.Meta.ResourceType)
			assert.Equal(t, "https://example.com/scim/v2/Users/"+user.ID, user.Meta.Location)
		})
	}

	t.Run("list", func(t *testing.T) {
		list := scimtest.Decode[protocol.ListResponse[core.User]](t, client.Get(t, basePath+"/Users"))

		require.NotEmpty(t, list.Resources)
		for _, user := range list.Resources {
			assert.Equal(t, core.ResourceTypeName("User"), user.Meta.ResourceType)
			assert.Equal(t, "https://example.com/scim/v2/Users/"+user.ID, user.Meta.Location)
		}
	})
}

func strip(user *core.User) *core.User {
	if user != nil {
		user.Meta.ResourceType, user.Meta.Location = "", ""
	}
	return user
}
