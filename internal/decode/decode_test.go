package decode_test

import (
	"bytes"
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/internal/decode"
)

var documents = map[string]string{
	"numbers":         `{"int":1,"float":1.5,"exp":-2e10,"list":[0,7]}`,
	"big int":         `{"id":12345678901234567890}`,
	"escaped keys":    `{"a\\b":"c\"d","é":"\nA"}`,
	"surrogate pair":  `{"emoji":"😀"}`,
	"lone surrogate":  `{"bad":"\ud800"}`,
	"null":            `{"x":null}`,
	"empty":           `{"a":[],"o":{}}`,
	"invalid utf8":    "{\"a\":\"b\xffc\"}",
	"escaped html":    `{"displayName":"R\u0026D \u003cops\u003e"}`,
	"booleans":        `[true,false]`,
	"top level null":  `null`,
	"top level value": ` "s" `,
}

var duplicates = map[string]string{
	"top level": `{"a":1,"a":2}`,
	"nested":    `{"o":{"a":1,"a":2}}`,
	"in array":  `[{"a":1,"a":2}]`,
	"escaped":   `{"a":1,"\u0061":2}`,
}

var malformed = map[string]string{
	"empty input":    ``,
	"whitespace":     " \n\t",
	"trailing data":  `{"a":1} {}`,
	"truncated":      `{"a":[1,`,
	"bad syntax":     `{"a" 1}`,
	"leading zero":   `{"a":01}`,
	"NaN":            `{"a":NaN}`,
	"trailing comma": `{"a":1,}`,
}

func TestValue(t *testing.T) {
	t.Run("matches JSON", func(t *testing.T) {
		for name, raw := range documents {
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
		for name, raw := range malformed {
			t.Run(name, func(t *testing.T) {
				_, err := decode.JSON[any]([]byte(raw))
				require.Error(t, err)

				_, err = decode.Value([]byte(raw))
				assert.Error(t, err)
			})
		}
	})

	t.Run("rejects duplicate names", func(t *testing.T) {
		for name, raw := range duplicates {
			t.Run(name, func(t *testing.T) {
				_, err := decode.Value([]byte(raw))
				assert.ErrorIs(t, err, jsontext.ErrDuplicateName)
			})
		}
	})

	t.Run("matches JSON at the nesting limit", func(t *testing.T) {
		for _, depth := range []int{9999, 10000, 10001} {
			t.Run(strconv.Itoa(depth), func(t *testing.T) {
				assertParity(t, []byte(strings.Repeat("[", depth)+strings.Repeat("]", depth)))
			})
		}
	})

	t.Run("does not alias the input", func(t *testing.T) {
		raw := []byte(`{"name":"before"}`)
		original := bytes.Clone(raw)

		got, err := decode.Value(raw)
		require.NoError(t, err)
		assert.Equal(t, original, raw)

		copy(raw, bytes.Repeat([]byte("x"), len(raw)))
		assert.Equal(t, map[string]any{"name": "before"}, got)
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

func FuzzValue(f *testing.F) {
	for _, raw := range documents {
		f.Add([]byte(raw))
	}
	for _, raw := range duplicates {
		f.Add([]byte(raw))
	}
	for _, raw := range malformed {
		f.Add([]byte(raw))
	}
	f.Fuzz(assertParity)
}

func BenchmarkValue(b *testing.B) {
	members := make([]map[string]string, 10000)
	for i := range members {
		id := strconv.Itoa(i)
		members[i] = map[string]string{"value": id, "$ref": "https://example.com/Users/" + id, "type": "User"}
	}
	raw, err := json.Marshal(map[string]any{"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:Group"}, "displayName": "engineering", "members": members})
	require.NoError(b, err)

	b.Run("Value", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = decode.Value(raw)
		}
	})
	b.Run("JSON", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			_, _ = decode.JSON[any](raw)
		}
	})
}

func assertParity(t *testing.T, raw []byte) {
	t.Helper()
	original := bytes.Clone(raw)
	want, wantErr := decode.JSON[any](raw)
	got, err := decode.Value(raw)
	assert.Equal(t, original, raw)
	if errors.Is(err, jsontext.ErrDuplicateName) {
		return
	}
	if wantErr != nil {
		assert.Error(t, err)
		return
	}
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
