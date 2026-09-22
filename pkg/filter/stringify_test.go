package filter_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/filter"
)

func TestStringify(t *testing.T) {
	attribute, err := filter.NewAttrPath("userName")
	require.NoError(t, err)

	t.Run("renders a string, bool, number, and null value", func(t *testing.T) {
		s, err := filter.Stringify{}.VisitEquals(attribute, "bjensen")
		require.NoError(t, err)
		assert.Equal(t, `userName eq "bjensen"`, s)

		s, err = filter.Stringify{}.VisitEquals(attribute, true)
		require.NoError(t, err)
		assert.Equal(t, `userName eq true`, s)

		s, err = filter.Stringify{}.VisitEquals(attribute, nil)
		require.NoError(t, err)
		assert.Equal(t, `userName eq null`, s)
	})

	t.Run("falls back to a plain %v for a value the grammar never produces", func(t *testing.T) {
		s, err := filter.Stringify{}.VisitEquals(attribute, 42)
		require.NoError(t, err)
		assert.Equal(t, `userName eq 42`, s)
	})

	t.Run("combines clauses and negation", func(t *testing.T) {
		and, err := filter.Stringify{}.VisitAnd("a", "b")
		require.NoError(t, err)
		assert.Equal(t, "(a and b)", and)

		or, err := filter.Stringify{}.VisitOr("a", "b")
		require.NoError(t, err)
		assert.Equal(t, "(a or b)", or)

		not, err := filter.Stringify{}.VisitNot("a")
		require.NoError(t, err)
		assert.Equal(t, "not (a)", not)
	})

	t.Run("renders presence and a value path", func(t *testing.T) {
		pr, err := filter.Stringify{}.VisitPresence(attribute)
		require.NoError(t, err)
		assert.Equal(t, "userName pr", pr)

		vp, err := filter.Stringify{}.VisitValuePath(attribute, "", func() (string, error) { return `type eq "work"`, nil })
		require.NoError(t, err)
		assert.Equal(t, `userName[type eq "work"]`, vp)

		vp, err = filter.Stringify{}.VisitValuePath(attribute, "value", func() (string, error) { return `type eq "work"`, nil })
		require.NoError(t, err)
		assert.Equal(t, `userName[type eq "work"].value`, vp)
	})
}
