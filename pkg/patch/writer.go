package patch

import (
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// RFC 7644 Section 3.5.2.3: sub-attributes that are not specified in the "value" parameter are left unchanged.
func merge(holder core.Object, values map[string]any, parent *core.Attribute, kind Op) {
	keys := newKeys(holder)
	for name, value := range values {
		key := keys.resolve(name)
		holder[key] = appended(holder[key], shaped(value, subAttr(parent, name).MultiValued), kind)
		keys.add(key)
	}
}

// RFC 7644 3.5.2.1 - a value written to a multi-valued attribute is an array.
// RFC 7643 Section 2.5: "null" for the value is treated the same as an empty array, not an array holding a null.
func shaped(value any, multiValued bool) any {
	if !multiValued {
		return value
	}
	if value == nil {
		return []any{}
	}
	if list, ok := value.([]any); ok {
		return list
	}
	return []any{value}
}

func set(o core.Object, name string, value any, kind Op) {
	o.Set(name, appended(o.Get(name), value, kind))
}

func appended(before, value any, kind Op) any {
	if list, ok := before.([]any); kind == OpAdd && ok {
		return append(list, shaped(value, true).([]any)...)
	}
	return value
}

func child(o core.Object, name string) (core.Object, error) {
	existing := o.Get(name)
	if existing == nil {
		fresh := map[string]any{}
		o.Set(name, fresh)
		return fresh, nil
	}
	nested, ok := existing.(map[string]any)
	if !ok {
		return nil, scimerrors.ErrInvalidPath(`"path" targets a non-complex attribute`)
	}
	return nested, nil
}
