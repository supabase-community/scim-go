package filter

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultGrammarBoundsInput(t *testing.T) {
	require.NotNil(t, DefaultGrammar)
	require.Positive(t, DefaultGrammar.MaxInputBytes)

	long := `userName eq "` + strings.Repeat("a", DefaultGrammar.MaxInputBytes) + `"`
	_, err := Parse(long)
	require.ErrorIs(t, err, ErrInputTooLarge)
}
