package server_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
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
