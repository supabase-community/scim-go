package server

import (
	"encoding/json"
	"time"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
)

type ownedKey string

// owners maps each value of a unique attribute to the id holding it, per RFC 7643, Section 7.
type owners struct {
	fields []field
	ids    []map[any]string
}

func newOwners(fields fields) owners {
	o := owners{}
	for _, field := range fields {
		if field.Uniqueness != core.UniquenessNone {
			o.fields = append(o.fields, field)
			o.ids = append(o.ids, map[any]string{})
		}
	}
	return o
}

func (o owners) conflict(id string, object core.Object) *core.Attribute {
	for i, field := range o.fields {
		for _, key := range uniqueKeys(field, object) {
			if owner, ok := o.ids[i][key]; ok && owner != id {
				return field.Attribute
			}
		}
	}
	return nil
}

func (o owners) claim(id string, object core.Object) {
	for i, field := range o.fields {
		for _, key := range uniqueKeys(field, object) {
			o.ids[i][key] = id
		}
	}
}

func (o owners) release(id string, object core.Object) {
	for i, field := range o.fields {
		for _, key := range uniqueKeys(field, object) {
			if o.ids[i][key] == id {
				delete(o.ids[i], key)
			}
		}
	}
}

// RFC 7644 Section 3.4.2.2: each value of a multi-valued attribute is compared on its own.
func uniqueKeys(field field, object core.Object) []any {
	keys := []any{}
	for _, v := range field.values(object) {
		if !value.IsUnassigned(v) {
			keys = append(keys, uniqueKey(field.Attribute, v))
		}
	}
	return keys
}

func uniqueKey(attribute *core.Attribute, v any) any {
	switch folded := value.Fold(attribute, v).(type) {
	case string, int64, float64, bool:
		return folded
	case time.Time:
		return folded.UTC()
	}
	encoded, _ := json.Marshal(v)
	return ownedKey(encoded)
}
