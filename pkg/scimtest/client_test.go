package scimtest_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
	"github.com/supabase-community/scim-go/pkg/scimtest"
	"github.com/supabase-community/scim-go/pkg/server"
)

func TestClient(t *testing.T) {
	handler := server.New("/scim/v2", core.NewServiceProviderConfig(),
		server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, core.UserAttributes()...)),
		server.WithAuthentication(core.NewOAuthBearerToken().AsPrimary(),
			server.RequireBearerToken(func(ctx context.Context, token string) (context.Context, error) {
				if token != "secret" {
					return ctx, server.ErrInvalidToken
				}
				return ctx, nil
			}),
		),
	)
	c := scimtest.Serve(t, handler, "/scim/v2", scimtest.WithBearerToken("secret"))

	t.Run("creates and reads a resource", func(t *testing.T) {
		created := c.Post(t, "/Users", core.User{UserName: "bjensen"})
		require.Equal(t, http.StatusCreated, created.StatusCode)
		user := scimtest.Decode[core.User](t, created)

		read := c.Get(t, "/Users/"+user.ID)
		require.Equal(t, http.StatusOK, read.StatusCode)
		assert.Equal(t, "bjensen", scimtest.Decode[core.User](t, read).UserName)
		assert.Equal(t, http.StatusNoContent, c.Delete(t, "/Users/"+user.ID).StatusCode)
	})

	t.Run("sends a byte body as is", func(t *testing.T) {
		scimtest.RequireError(t, c.Post(t, "/Users", []byte(`{"userName":`)), http.StatusBadRequest, scimerrors.InvalidSyntax)
	})

	t.Run("lets a copy override a header without changing the original", func(t *testing.T) {
		scimtest.RequireError(t, c.With(scimtest.WithBearerToken("revoked")).Get(t, "/Users"), http.StatusUnauthorized, "")
		assert.Equal(t, http.StatusOK, c.Get(t, "/Users").StatusCode)
	})

	t.Run("returns a SCIM error for an unknown id", func(t *testing.T) {
		scimtest.RequireError(t, c.Send(t, http.MethodGet, "/Users/missing", nil), http.StatusNotFound, "")
	})
}
