package memory

import (
	"encoding/json"
	"time"

	"github.com/supabase-community/scim-go/internal/fields"
	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
)

type indexKey struct {
	scope     *core.Schema
	attribute *core.Attribute
	value     any
}

func indexKeysOf(e entry, unique fields.Fields) []indexKey {
	keys := []indexKey{}
	for _, field := range unique {
		if field.Uniqueness == core.UniquenessNone {
			continue
		}
		for _, v := range uniqueKeys(field, e.body) {
			keys = append(keys, indexKey{scope: e.schema, attribute: field.Attribute, value: v})
		}
	}
	return keys
}

func uniqueKeys(field fields.Field, object core.Object) []any {
	keys := []any{}
	for _, v := range field.Values(object) {
		if !value.IsUnassigned(v) {
			keys = append(keys, uniqueKey(field.Attribute, v))
		}
	}
	return keys
}

func uniqueKey(attribute *core.Attribute, v any) any {
	switch folded := attribute.Fold(v).(type) {
	case string, int64, float64, bool:
		return folded
	case time.Time:
		return folded.UTC()
	}
	encoded, _ := json.Marshal(v)
	return encodedValue(encoded)
}
