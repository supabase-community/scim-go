package scim

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/go-scim/pkg/core"
)

func TestSchema(t *testing.T) {
	baseURL := "http://example.com/scim/v2"

	t.Run("NewSchema", func(t *testing.T) {
		schema := NewSchema(baseURL, KindUser)

		t.Run("identifies itself by the URI of the kind it describes", func(t *testing.T) {
			require.Equal(t, []core.SchemaURI{core.SchemaSchema}, schema.Schemas)
			require.Equal(t, core.SchemaUser, schema.ID)
			require.Equal(t, KindUser.Name, schema.Name)
		})

		t.Run("locates itself by URI under the Schemas endpoint", func(t *testing.T) {
			require.Equal(t, KindSchema.Name, schema.Meta.ResourceType)
			require.Equal(t, baseURL+"/Schemas/urn:ietf:params:scim:schemas:core:2.0:User", schema.Meta.Location)
		})
	})

	t.Run("Describe", func(t *testing.T) {
		schema := NewSchema(baseURL, KindUser)

		require.Same(t, schema, schema.Describe("User Account"))
		assert.Equal(t, "User Account", schema.Description)
	})

	t.Run("With", func(t *testing.T) {
		userName := core.NewAttribute("userName", core.TypeString, "A unique identifier for the user.")
		schema := NewSchema(baseURL, KindUser)

		require.Same(t, schema, schema.With(userName))
		assert.Equal(t, []*core.Attribute{userName}, schema.Attributes)
	})
}
