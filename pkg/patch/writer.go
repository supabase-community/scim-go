package patch

import (
	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// RFC 7643 Section 2.5: the null value or an empty array SHALL be considered equivalent for a multi-valued attribute.
func shaped(attribute *core.Attribute, incoming any) any {
	if !attribute.MultiValued {
		return incoming
	}
	return value.Unique(attribute, shapedList(incoming))
}

func shapedList(incoming any) []any {
	switch list := incoming.(type) {
	case nil:
		return []any{}
	case []any:
		return list
	}
	return []any{incoming}
}

func set(o core.Object, name string, incoming any, kind Op) {
	o.Set(name, appended(o.Get(name), incoming, kind))
}

func appended(before, incoming any, kind Op) any {
	if list, ok := before.([]any); kind == OpAdd && ok {
		return append(list, shapedList(incoming)...)
	}
	return incoming
}

func child(o core.Object, name string) (core.Object, error) {
	existing := o.Get(name)
	if existing == nil {
		fresh := map[string]any{}
		o.Set(name, fresh)
		return fresh, nil
	}
	nested := value.AsObject(existing)
	if nested == nil {
		return nil, scimerrors.ErrInvalidPath(`"path" targets a non-complex attribute`)
	}
	return nested, nil
}
