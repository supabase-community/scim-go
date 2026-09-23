package peg_test

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/filter/internal/peg"
)

func TestSequence(t *testing.T) {
	t.Run("returns true", func(t *testing.T) {
		atom := peg.Sequence(
			peg.Str("userName"),
			peg.Space(),
			peg.Str("eq"),
			peg.Space(),
			peg.Match(regexp.MustCompile(`"[a-z]+"`)),
		)
		ctx := peg.NewContext(`userName eq "bjensen"`)
		result, err := atom(ctx)

		require.NoError(t, err)
		assert.Equal(t, 21, ctx.Position())
		assert.Equal(t, []peg.ASTNode{"userName", "eq", `"bjensen"`}, result)
	})

	t.Run("returns false when a parser fails, resetting position", func(t *testing.T) {
		atom := peg.Sequence(
			peg.Str("userName"),
			peg.Space(),
			peg.Str("ne"),
		)
		ctx := peg.NewContext(`userName eq "bjensen"`)
		result, err := atom(ctx)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, 0, ctx.Position())
	})

	t.Run("collects non-nil results while skipping nil ones", func(t *testing.T) {
		atom := peg.Sequence(
			peg.Str("a"),
			peg.Space(),
			peg.Str("b"),
		)
		ctx := peg.NewContext("a b")
		result, err := atom(ctx)

		require.NoError(t, err)
		assert.Equal(t, 3, ctx.Position())
		assert.Equal(t, []peg.ASTNode{"a", "b"}, result)
	})

	t.Run("merges tagged results into a single Token", func(t *testing.T) {
		atom := peg.Sequence(
			peg.Tag("a", peg.Str("x")),
			peg.Str(","),
			peg.Tag("b", peg.Str("y")),
		)
		ctx := peg.NewContext("x,y")
		result, err := atom(ctx)

		require.NoError(t, err)
		assert.Equal(t, peg.Token{"a": "x", "b": "y"}, result)
	})

	t.Run("drops untagged results when merging, keeps only Token entries", func(t *testing.T) {
		atom := peg.Sequence(
			peg.Str("("),
			peg.Space(),
			peg.Tag("attribute", peg.Str("userName")),
			peg.Space(),
			peg.Str(")"),
		)
		ctx := peg.NewContext("( userName )")
		result, err := atom(ctx)

		require.NoError(t, err)
		assert.Equal(t, peg.Token{"attribute": "userName"}, result)
	})
}
