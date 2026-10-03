package filter_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/filter"
)

func TestAttrPathKey(t *testing.T) {
	tt := []struct {
		path filter.AttrPath
		want string
	}{
		{filter.AttrPath{Name: "userName"}, "username"},
		{filter.AttrPath{Name: "meta", SubAttribute: "lastModified"}, "meta.lastmodified"},
		{filter.AttrPath{URI: "urn:ietf:params:scim:schemas:core:2.0:User", Name: "userName"}, "username"},
	}
	for _, tc := range tt {
		t.Run(tc.want, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.path.Key())
		})
	}
}

func TestNewAttrPath(t *testing.T) {
	t.Run("parses a bare attribute", func(t *testing.T) {
		p, err := filter.NewAttrPath("userName")
		require.NoError(t, err)
		assert.Equal(t, "userName", p.Name)
		assert.Empty(t, p.URI)
		assert.Empty(t, p.SubAttribute)
	})

	t.Run("parses a dotted sub-attribute", func(t *testing.T) {
		p, err := filter.NewAttrPath("name.familyName")
		require.NoError(t, err)
		assert.Equal(t, "name", p.Name)
		assert.Equal(t, "familyName", p.SubAttribute)
	})

	t.Run("parses a multi-colon schema URN", func(t *testing.T) {
		p, err := filter.NewAttrPath("urn:ietf:params:scim:schemas:core:2.0:User:userName")
		require.NoError(t, err)
		assert.Equal(t, "urn:ietf:params:scim:schemas:core:2.0:User", p.URI)
		assert.Equal(t, "userName", p.Name)
	})

	t.Run("rejects malformed attr paths", func(t *testing.T) {
		for _, text := range []string{
			"", "123bad", "name.", ".name", "name.familyName.extra",
			"a_b:name", "foo bar:department", "userName ", `emails[type eq "work"]`,
		} {
			_, err := filter.NewAttrPath(text)
			require.Error(t, err, text)
		}
	})
}
