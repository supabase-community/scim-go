package filter_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/filter"
)

func TestParseErrorMessage(t *testing.T) {
	g := filter.New(0)
	_, err := g.Parse(`userName eq`)

	var perr *filter.ParseError
	require.ErrorAs(t, err, &perr)
	assert.Contains(t, perr.Error(), "scim: invalid filter at position")
}

func TestParseErrorReportsPosition(t *testing.T) {
	g := filter.New(0)
	input := `userName eq "bjensen" garbage`
	_, err := g.Parse(input)

	require.ErrorIs(t, err, filter.ErrInvalidFilter)
	var perr *filter.ParseError
	require.ErrorAs(t, err, &perr)
	assert.Equal(t, input, perr.Input)
	assert.Equal(t, len(`userName eq "bjensen"`), perr.Position)
}
