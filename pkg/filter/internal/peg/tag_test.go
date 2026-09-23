package peg_test

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/filter/internal/peg"
)

func TestTag(t *testing.T) {
	t.Run("returns true", func(t *testing.T) {
		atom := peg.Tag("filter", peg.Sequence(
			peg.Str("userName"),
			peg.Space(),
			peg.Str("eq"),
			peg.Space(),
			peg.Match(regexp.MustCompile(`"[a-z]+"`)),
		))
		ctx := peg.NewContext(`userName eq "bjensen"`)
		result, err := atom(ctx)

		require.NoError(t, err)
		assert.Equal(t, 21, ctx.Position())
		assert.Equal(t, peg.Token{"filter": []peg.ASTNode{"userName", "eq", `"bjensen"`}}, result)
	})

	t.Run("returns false when inner parser fails", func(t *testing.T) {
		atom := peg.Tag("filter", peg.Str("doesnotexist"))
		ctx := peg.NewContext(`userName eq "bjensen"`)
		result, err := atom(ctx)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Equal(t, 0, ctx.Position())
	})
}
