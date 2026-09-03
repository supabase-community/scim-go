package peg

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSequence(t *testing.T) {
	t.Run("returns true", func(t *testing.T) {
		atom := Sequence(
			Str("userName"),
			Space(),
			Str("eq"),
			Space(),
			Match(regexp.MustCompile(`"[a-z]+"`)),
		)
		ctx := NewContext(`userName eq "bjensen"`)
		result, err := atom(ctx)

		assert.NoError(t, err)
		assert.Equal(t, 21, ctx.position)
		assert.Equal(t, []ASTNode{"userName", "eq", `"bjensen"`}, result)
	})

	t.Run("returns false when a parser fails, resetting position", func(t *testing.T) {
		atom := Sequence(
			Str("userName"),
			Space(),
			Str("ne"),
		)
		ctx := &Context{stream: `userName eq "bjensen"`}
		result, err := atom(ctx)

		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, 0, ctx.position)
	})

	t.Run("collects non-nil results while skipping nil ones", func(t *testing.T) {
		atom := Sequence(
			Str("a"),
			Space(),
			Str("b"),
		)
		ctx := &Context{stream: "a b"}
		result, err := atom(ctx)

		assert.NoError(t, err)
		assert.Equal(t, 3, ctx.position)
		assert.Equal(t, []ASTNode{"a", "b"}, result)
	})

	t.Run("merges tagged results into a single Token", func(t *testing.T) {
		atom := Sequence(
			Tag("a", Str("x")),
			Str(","),
			Tag("b", Str("y")),
		)
		ctx := NewContext("x,y")
		result, err := atom(ctx)

		assert.NoError(t, err)
		assert.Equal(t, Token{"a": "x", "b": "y"}, result)
	})

	t.Run("drops untagged results when merging, keeps only Token entries", func(t *testing.T) {
		atom := Sequence(
			Str("("),
			Space(),
			Tag("attribute", Str("userName")),
			Space(),
			Str(")"),
		)
		ctx := NewContext("( userName )")
		result, err := atom(ctx)

		assert.NoError(t, err)
		assert.Equal(t, Token{"attribute": "userName"}, result)
	})
}
