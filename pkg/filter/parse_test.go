package filter

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultGrammarBoundsInput(t *testing.T) {
	require.NotNil(t, DefaultGrammar)
	require.Positive(t, defaultGrammar.maxInputBytes)

	long := `userName eq "` + strings.Repeat("a", defaultGrammar.maxInputBytes) + `"`
	_, err := Parse(long)
	require.ErrorIs(t, err, ErrInputTooLarge)
}
