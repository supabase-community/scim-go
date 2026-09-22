package core_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
)

func TestCommonAttribute(t *testing.T) {
	t.Run("finds a common attribute by name, per RFC 7643 Section 3.1", func(t *testing.T) {
		attribute, ok := core.CommonAttribute("meta")
		require.True(t, ok)
		assert.Equal(t, core.TypeComplex, attribute.Type)
		assert.NotNil(t, attribute.SubAttributes.Lookup("lastModified"))
	})

	t.Run("reports false for a name that is not common", func(t *testing.T) {
		_, ok := core.CommonAttribute("userName")
		assert.False(t, ok)
	})
}
