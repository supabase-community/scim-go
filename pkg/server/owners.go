package server

import (
	"encoding/json"
	"time"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
)

type ownedKey string

// owners maps each value of a unique attribute to the id holding it, per RFC 7643, Section 7.
type owners []claims

type claims struct {
	field
	ids map[any]string
}

func newOwners(fields fields) owners {
	o := owners{}
	for _, field := range fields {
		if field.Uniqueness != core.UniquenessNone {
			o = append(o, claims{field: field, ids: map[any]string{}})
		}
	}
	return o
}

func (o owners) conflict(id string, object core.Object) *core.Attribute {
	var found *core.Attribute
	o.each(object, func(c claims, key any) bool {
		if owner, ok := c.ids[key]; ok && owner != id {
			found = c.Attribute
			return true
		}
		return false
	})
	return found
}

func (o owners) claim(id string, object core.Object) {
	o.each(object, func(c claims, key any) bool {
		c.ids[key] = id
		return false
	})
}

func (o owners) release(id string, object core.Object) {
	o.each(object, func(c claims, key any) bool {
		if c.ids[key] == id {
			delete(c.ids, key)
		}
		return false
	})
}

// each walks every unique-attribute claim key for object, stopping early when fn returns true.
func (o owners) each(object core.Object, fn func(c claims, key any) bool) {
	for _, c := range o {
		for _, key := range uniqueKeys(c.field, object) {
			if fn(c, key) {
				return
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
