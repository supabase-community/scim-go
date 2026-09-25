package server

import (
	"slices"

	"github.com/supabase-community/scim-go/pkg/core"
)

type field struct {
	*core.Attribute
	parent *core.Attribute
	raw    func(core.Object) any
}

type fields []field

func newFields(schemas core.Schemas) fields {
	f := fields{}
	root := func(d core.Object) any { return d }
	for _, name := range []string{"id", "externalId", "meta"} {
		attribute, _ := core.CommonAttribute(name)
		f = f.with(root, attribute)
	}
	f = f.with(root, schemas.Base().Attributes...)
	for _, extension := range schemas.Extensions() {
		f = f.with(func(d core.Object) any { return d.Get(string(extension.ID)) }, extension.Attributes...)
	}
	return f
}

func (f fields) with(parent func(core.Object) any, attributes ...*core.Attribute) fields {
	for _, attribute := range attributes {
		top := field{Attribute: attribute, raw: func(d core.Object) any { return asObject(parent(d)).Get(attribute.Name) }}
		f = append(f, top)
		for _, sub := range attribute.SubAttributes {
			f = append(f, top.sub(sub))
		}
	}
	return f
}

func (f fields) lookup(attribute *core.Attribute) (field, bool) {
	i := slices.IndexFunc(f, func(candidate field) bool { return candidate.Attribute == attribute })
	if i < 0 {
		return field{}, false
	}
	return f[i], true
}

func (f field) value(d core.Object) any {
	return coerce(f.Attribute, f.raw(d))
}

func (f field) elements(d core.Object) []any {
	list, _ := f.raw(d).([]any)
	return list
}

func (f field) isList() bool {
	return f.parent == nil && f.MultiValued
}

// RFC 7644 Section 3.4.2.2: outside a value path, a sub-attribute holds the values of every element.
func (f field) sub(sub *core.Attribute) field {
	if !f.MultiValued {
		return field{Attribute: sub, parent: f.Attribute, raw: func(d core.Object) any { return asObject(f.raw(d)).Get(sub.Name) }}
	}
	return field{Attribute: sub, parent: f.Attribute, raw: func(d core.Object) any {
		list := f.elements(d)
		values := make([]any, len(list))
		for i, element := range list {
			values[i] = asObject(element).Get(sub.Name)
		}
		return values
	}}
}

func asObject(value any) core.Object {
	switch v := value.(type) {
	case core.Object:
		return v
	case map[string]any:
		return v
	}
	return nil
}

func coerce(attribute *core.Attribute, value any) any {
	if attribute.Type == core.TypeComplex {
		return value
	}
	list, ok := value.([]any)
	if !ok {
		return coerced(attribute, value)
	}
	values := make([]any, len(list))
	for i, element := range list {
		values[i] = coerced(attribute, element)
	}
	return values
}

func coerced(attribute *core.Attribute, value any) any {
	if typed, ok := attribute.Coerce(value); ok {
		return typed
	}
	return nil
}
