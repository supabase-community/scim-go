package value_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/supabase-community/scim-go/internal/value"
)

func TestIsUnassigned(t *testing.T) {
	for _, v := range []any{nil, "", []any{}, map[string]any{}} {
		assert.True(t, value.IsUnassigned(v), "%#v", v)
	}
	for _, v := range []any{"a", false, json.Number("0"), []any{nil}, map[string]any{"a": nil}} {
		assert.False(t, value.IsUnassigned(v), "%#v", v)
	}
}
