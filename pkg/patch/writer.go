package patch

import (
	"strconv"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// RFC 7644 Section 3.5.2.3: sub-attributes that are not specified in the "value" parameter are left unchanged.
func merge(holder core.Object, values map[string]any, parent *core.Attribute, kind Op) error {
	keys := newKeys(holder)
	for name, incoming := range values {
		key := keys.resolve(name)
		sub := subAttr(parent, name)
		v := shaped(incoming, sub.MultiValued)
		if existing, ok := holder[key].([]any); kind == OpAdd && sub.MultiValued && ok {
			v = freshElements(sub, existing, v.([]any))
		}
		if err := gateImmutableWrite(sub, holder[key], appended(holder[key], v, kind)); err != nil {
			return err
		}
		holder[key] = appended(holder[key], v, kind)
		keys.add(key)
	}
	return nil
}

// RFC 7643 Section 7: an assigned immutable sub-attribute rejects any write that would change it.
func gateImmutableWrite(sub *core.Attribute, before, candidate any) error {
	if sub.Mutability != core.MutabilityImmutable || value.IsUnassigned(before) || value.Equal(sub, before, candidate) {
		return nil
	}
	return scimerrors.ErrMutability(strconv.Quote(sub.Name) + " is immutable")
}

// RFC 7644 3.5.2.1 - a value written to a multi-valued attribute is an array; RFC 7643 Section 2.5 treats "null" the same as an empty array.
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
