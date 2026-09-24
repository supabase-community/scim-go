package core_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/supabase-community/scim-go/pkg/core"
)

func TestObject(t *testing.T) {
	t.Run("gets a value by a case-insensitive name", func(t *testing.T) {
		object := core.Object{"userName": "bjensen"}
		assert.True(t, object.Has("USERNAME"))
		assert.Equal(t, "bjensen", object.Get("USERNAME"))
	})

	t.Run("reports a missing name", func(t *testing.T) {
		assert.False(t, core.Object{}.Has("userName"))
		assert.Nil(t, core.Object{}.Get("userName"))
	})

	t.Run("prefers the exact name, then the smallest case-insensitive match", func(t *testing.T) {
		object := core.Object{"USERNAME": "a", "userName": "b", "UserName": "c"}
		assert.Equal(t, "b", object.Get("userName"))
		assert.Equal(t, "a", object.Get("username"))
	})

	t.Run("sets a value under the existing name", func(t *testing.T) {
		object := core.Object{"userName": "bjensen"}
		object.Set("USERNAME", "jsmith")
		object.Set("nickName", "Babs")
		assert.Equal(t, core.Object{"userName": "jsmith", "nickName": "Babs"}, object)
	})

	t.Run("removes a value by a case-insensitive name", func(t *testing.T) {
		object := core.Object{"userName": "bjensen"}
		object.Remove("USERNAME")
		assert.Empty(t, object)
	})
}
