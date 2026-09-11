package filter_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/filter"
)

func TestParsePath(t *testing.T) {
	t.Run("parses a bare attribute", func(t *testing.T) {
		p, err := filter.ParsePath("userName")
		require.NoError(t, err)
		assert.Equal(t, "userName", p.Name)
		assert.Empty(t, p.SubAttribute)
		assert.Nil(t, p.ValueFilter)
	})

	t.Run("parses a dotted sub-attribute", func(t *testing.T) {
		p, err := filter.ParsePath("name.familyName")
		require.NoError(t, err)
		assert.Equal(t, "name", p.Name)
		assert.Equal(t, "familyName", p.SubAttribute)
		assert.Nil(t, p.ValueFilter)
	})

	t.Run("parses a schema-qualified attribute", func(t *testing.T) {
		p, err := filter.ParsePath("urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department")
		require.NoError(t, err)
		assert.Equal(t, "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User", p.URI)
		assert.Equal(t, "department", p.Name)
	})

	t.Run("parses a value path", func(t *testing.T) {
		p, err := filter.ParsePath(`emails[type eq "work"]`)
		require.NoError(t, err)
		assert.Equal(t, "emails", p.Name)
		assert.Empty(t, p.SubAttribute)
		require.NotNil(t, p.ValueFilter)
	})

	t.Run("parses a value path with a sub-attribute", func(t *testing.T) {
		p, err := filter.ParsePath(`emails[type eq "work"].value`)
		require.NoError(t, err)
		assert.Equal(t, "emails", p.Name)
		assert.Equal(t, "value", p.SubAttribute)
		require.NotNil(t, p.ValueFilter)
	})

	t.Run("rejects malformed paths", func(t *testing.T) {
		for _, text := range []string{"", "123bad", "emails[", `emails[type eq "work"`, "name.", ".name"} {
			_, err := filter.ParsePath(text)
			require.Error(t, err, text)
		}
	})
}
