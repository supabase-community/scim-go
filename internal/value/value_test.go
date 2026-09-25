package value_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
)

func TestIsUnassigned(t *testing.T) {
	for _, v := range []any{nil, "", []any{}, map[string]any{}} {
		assert.True(t, value.IsUnassigned(v), "%#v", v)
	}
	for _, v := range []any{"a", false, json.Number("0"), []any{nil}, map[string]any{"a": nil}} {
		assert.False(t, value.IsUnassigned(v), "%#v", v)
	}
}

func TestFold(t *testing.T) {
	t.Run("lowercases a string that is not case exact", func(t *testing.T) {
		assert.Equal(t, "bjensen", value.Fold(core.NewAttribute("userName", core.TypeString), "BJensen"))
	})

	t.Run("preserves a case exact string", func(t *testing.T) {
		assert.Equal(t, "BJensen", value.Fold(core.NewAttribute("id", core.TypeString).AsCaseExact(), "BJensen"))
	})

	// RFC 7643 Sections 2.3.6 and 2.3.7: binary and reference values are case exact.
	t.Run("preserves binary and reference values", func(t *testing.T) {
		assert.Equal(t, "QUJD", value.Fold(core.NewAttribute("x509", core.TypeBinary), "QUJD"))
		assert.Equal(t, "https://Example.com/Users/A", value.Fold(core.NewAttribute("$ref", core.TypeReference), "https://Example.com/Users/A"))
	})
}

func TestCompare(t *testing.T) {
	t.Run("does not order values of different types", func(t *testing.T) {
		for _, pair := range [][2]any{{"a", int64(1)}, {true, "a"}, {time.Now(), "a"}, {nil, nil}} {
			order, ok := value.Compare(pair[0], pair[1])
			assert.False(t, ok, "%#v", pair)
			assert.Zero(t, order, "%#v", pair)
		}
	})
}
