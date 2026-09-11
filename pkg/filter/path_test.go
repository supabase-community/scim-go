package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePath(t *testing.T) {
	t.Run("parses a bare attribute", func(t *testing.T) {
		p, err := ParsePath("userName")
		require.NoError(t, err)
		assert.Equal(t, "userName", p.Name)
		assert.Empty(t, p.SubAttribute)
		assert.Nil(t, p.ValueFilter)
	})

	t.Run("parses a dotted sub-attribute", func(t *testing.T) {
		p, err := ParsePath("name.familyName")
		require.NoError(t, err)
		assert.Equal(t, "name", p.Name)
		assert.Equal(t, "familyName", p.SubAttribute)
		assert.Nil(t, p.ValueFilter)
	})

	t.Run("parses a schema-qualified attribute", func(t *testing.T) {
		p, err := ParsePath("urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department")
		require.NoError(t, err)
		assert.Equal(t, "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User", p.URI)
		assert.Equal(t, "department", p.Name)
	})

	t.Run("parses a value path", func(t *testing.T) {
		p, err := ParsePath(`emails[type eq "work"]`)
		require.NoError(t, err)
		assert.Equal(t, "emails", p.Name)
		assert.Empty(t, p.SubAttribute)
		require.NotNil(t, p.ValueFilter)
	})

	t.Run("parses a value path with a sub-attribute", func(t *testing.T) {
		p, err := ParsePath(`emails[type eq "work"].value`)
		require.NoError(t, err)
		assert.Equal(t, "emails", p.Name)
		assert.Equal(t, "value", p.SubAttribute)
		require.NotNil(t, p.ValueFilter)
	})

	t.Run("rejects malformed paths", func(t *testing.T) {
		for _, text := range []string{
			"", "123bad", "emails[", `emails[type eq "work"`, "name.", ".name",
			"name.familyName.extra", "a_b:name", "foo bar:department", "userName ",
		} {
			_, err := ParsePath(text)
			require.Error(t, err, text)
		}
	})
}

func TestParseAttrPath(t *testing.T) {
	t.Run("parses a bare attribute", func(t *testing.T) {
		p, err := ParseAttrPath("userName")
		require.NoError(t, err)
		assert.Equal(t, "userName", p.Name)
		assert.Empty(t, p.URI)
		assert.Empty(t, p.SubAttribute)
	})

	t.Run("parses a dotted sub-attribute", func(t *testing.T) {
		p, err := ParseAttrPath("name.familyName")
		require.NoError(t, err)
		assert.Equal(t, "name", p.Name)
		assert.Equal(t, "familyName", p.SubAttribute)
	})

	t.Run("parses a multi-colon schema URN", func(t *testing.T) {
		p, err := ParseAttrPath("urn:ietf:params:scim:schemas:core:2.0:User:userName")
		require.NoError(t, err)
		assert.Equal(t, "urn:ietf:params:scim:schemas:core:2.0:User", p.URI)
		assert.Equal(t, "userName", p.Name)
	})

	t.Run("rejects malformed attr paths", func(t *testing.T) {
		for _, text := range []string{
			"", "123bad", "name.", ".name", "name.familyName.extra",
			"a_b:name", "foo bar:department", "userName ", `emails[type eq "work"]`,
		} {
			_, err := ParseAttrPath(text)
			require.Error(t, err, text)
		}
	})
}
