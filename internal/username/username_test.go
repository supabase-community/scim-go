package username_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/internal/username"
)

func TestPrepare(t *testing.T) {
	t.Run("maps case, width and composition", func(t *testing.T) {
		for input, want := range map[string]string{
			"Émile.X":                   "émile.x",
			"ＢJensen":                   "bjensen",
			"émile":                    "émile",
			"BJensen":                   "bjensen",
			"Barbara Jensen":            "barbara jensen",
			"Barbara  Jensen":           "barbara  jensen",
			"bjensen@example.com":       "bjensen@example.com",
			"0190-pr\u00e9@example.com": "0190-pr\u00e9@example.com",
			"1\u00e9":                   "1\u00e9",
			"\u05d0\u05d1":              "\u05d0\u05d1",
			"\u0627\u0644\u0639\u0631\u0628\u064a\u0629": "\u0627\u0644\u0639\u0631\u0628\u064a\u0629",
		} {
			got, err := username.Prepare(input, false)
			require.NoError(t, err, "%q", input)
			assert.Equal(t, want, got, "%q", input)
		}
	})

	t.Run("preserves case when case exact", func(t *testing.T) {
		got, err := username.Prepare("ＢJensen", true)
		require.NoError(t, err)
		assert.Equal(t, "BJensen", got)
	})

	t.Run("rejects a string that is not a username", func(t *testing.T) {
		for _, input := range []string{"", " bjensen", "bjensen ", "Bad\u0000Name", "tab\tname", "BJensen\U0001F600", "1\u05d0", "\u05d0a"} {
			_, err := username.Prepare(input, false)
			assert.Error(t, err, "%q", input)
		}
	})
}
