package peg

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOptional(t *testing.T) {
	t.Run("returns the wrapped value", func(t *testing.T) {
		atom := Optional(Str("foo"))
		ctx := NewContext("foobar")
		result, err := atom(ctx)

		assert.NoError(t, err)
		assert.Equal(t, "foo", result)
		assert.Equal(t, 3, ctx.position)
	})

	t.Run("succeeds with nil", func(t *testing.T) {
		atom := Optional(Str("foo"))
		ctx := NewContext("barfoo")
		result, err := atom(ctx)

		assert.NoError(t, err)
		assert.Nil(t, result)
		assert.Equal(t, 0, ctx.position)
	})

	t.Run("composes inside a Sequence", func(t *testing.T) {
		atom := Sequence(Str("a"), Optional(Str("b")), Str("c"))
		ctx := NewContext("ac")
		result, err := atom(ctx)

		assert.NoError(t, err)
		assert.Equal(t, []ASTNode{"a", "c"}, result)
	})
}
