package peg_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/filter/internal/peg"
)

func TestOptional(t *testing.T) {
	t.Run("returns the wrapped value", func(t *testing.T) {
		atom := peg.Optional(peg.Str("foo"))
		ctx := peg.NewContext("foobar")
		result, err := atom(ctx)

		require.NoError(t, err)
		assert.Equal(t, "foo", result)
		assert.Equal(t, 3, ctx.Position())
	})

	t.Run("succeeds with nil", func(t *testing.T) {
		atom := peg.Optional(peg.Str("foo"))
		ctx := peg.NewContext("barfoo")
		result, err := atom(ctx)

		require.NoError(t, err)
		assert.Nil(t, result)
		assert.Equal(t, 0, ctx.Position())
	})

	t.Run("composes inside a Sequence", func(t *testing.T) {
		atom := peg.Sequence(peg.Str("a"), peg.Optional(peg.Str("b")), peg.Str("c"))
		ctx := peg.NewContext("ac")
		result, err := atom(ctx)

		require.NoError(t, err)
		assert.Equal(t, []peg.ASTNode{"a", "c"}, result)
	})
}
