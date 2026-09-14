package filter_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/filter"
)

func TestDefaultGrammarBoundsInput(t *testing.T) {
	require.NotNil(t, filter.DefaultGrammar)

	long := `userName eq "` + strings.Repeat("a", 8192) + `"`
	_, err := filter.Parse(long)
	require.ErrorIs(t, err, filter.ErrInputTooLarge)
}
