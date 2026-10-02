package decode_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/internal/decode"
)

func TestValue(t *testing.T) {
	t.Run("matches JSON", func(t *testing.T) {
		for name, raw := range map[string]string{
			"numbers":         `{"int":1,"float":1.5,"exp":-2e10,"list":[0,7]}`,
			"escaped keys":    `{"a\\b":"c\"d","é":"\nA"}`,
			"null":            `{"x":null}`,
			"empty":           `{"a":[],"o":{}}`,
			"duplicate keys":  `{"a":1,"a":2}`,
			"invalid utf8":    "{\"a\":\"b\xffc\"}",
			"booleans":        `[true,false]`,
			"top level null":  `null`,
			"top level value": ` "s" `,
		} {
			t.Run(name, func(t *testing.T) {
				want, err := decode.JSON[any]([]byte(raw))
				require.NoError(t, err)

				got, err := decode.Value([]byte(raw))
				require.NoError(t, err)
				assert.Equal(t, want, got)
			})
		}
	})

	t.Run("rejects", func(t *testing.T) {
		for name, raw := range map[string]string{
			"empty input":   ``,
			"trailing data": `{"a":1} {}`,
			"truncated":     `{"a":[1,`,
			"bad syntax":    `{"a" 1}`,
		} {
			t.Run(name, func(t *testing.T) {
				_, err := decode.Value([]byte(raw))
				assert.Error(t, err)
			})
		}
	})
}

func TestObject(t *testing.T) {
	t.Run("returns a JSON object", func(t *testing.T) {
		got, err := decode.Object([]byte(`{"a":1}`))
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"a": json.Number("1")}, got)
	})

	t.Run("rejects", func(t *testing.T) {
		for _, raw := range []string{`null`, `[]`, `"s"`, `1`, `{`} {
			t.Run(raw, func(t *testing.T) {
				_, err := decode.Object([]byte(raw))
				assert.Error(t, err)
			})
		}
	})
}
