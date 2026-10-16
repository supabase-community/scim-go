package fields

import (
	"slices"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
)

type Field struct {
	*core.Attribute
	Parent *core.Attribute
	Raw    func(core.Object) any
	Holder func(core.Object) any
}

type Fields []Field

func New(schemas core.Schemas) Fields {
	f := Fields{}
	root := func(d core.Object) any { return d }
	for _, name := range []string{"schemas", "id", "externalId", "meta"} {
		attribute, _ := core.CommonAttribute(name)
		f = f.With(root, attribute)
	}
	f = f.With(root, schemas.Base().Attributes...)
	for _, extension := range schemas.Extensions() {
		f = f.With(func(d core.Object) any { return d.Get(string(extension.ID)) }, extension.Attributes...)
	}
	return f
}

func (f Fields) With(parent func(core.Object) any, attributes ...*core.Attribute) Fields {
	for _, attribute := range attributes {
		top := Field{Attribute: attribute, Holder: parent, Raw: func(d core.Object) any { return value.AsObject(parent(d)).Get(attribute.Name) }}
		f = append(f, top)
		for _, sub := range attribute.SubAttributes {
			f = append(f, top.Sub(sub))
		}
	}
	return f
}

func (f Fields) Lookup(attribute *core.Attribute) (Field, bool) {
	i := slices.IndexFunc(f, func(candidate Field) bool { return candidate.Attribute == attribute })
	if i < 0 {
		return Field{}, false
	}
	return f[i], true
}

func (f Field) Value(d core.Object) any {
	return Coerce(f.Attribute, f.Raw(d))
}

func (f Field) Elements(d core.Object) []any {
	list, _ := f.Raw(d).([]any)
	return list
}

func (f Field) Values(d core.Object) []any {
	return ValuesOf(f.Value(d))
}

func (f Field) IsList() bool {
	return f.Parent == nil && f.MultiValued
}

// Sub follows RFC 7644 Section 3.4.2.2: outside a value path, a sub-attribute holds the values of every element.
func (f Field) Sub(sub *core.Attribute) Field {
	if !f.MultiValued {
		return Field{Attribute: sub, Parent: f.Attribute, Holder: f.Raw, Raw: func(d core.Object) any { return value.AsObject(f.Raw(d)).Get(sub.Name) }}
	}
	return Field{Attribute: sub, Parent: f.Attribute, Holder: f.Raw, Raw: func(d core.Object) any {
		list := f.Elements(d)
		values := make([]any, len(list))
		for i, element := range list {
			values[i] = value.AsObject(element).Get(sub.Name)
		}
		return values
	}}
}

func ValuesOf(raw any) []any {
	if list, ok := raw.([]any); ok {
		return list
	}
	return []any{raw}
}

func Coerce(attribute *core.Attribute, raw any) any {
	if attribute.Type == core.TypeComplex {
		return raw
	}
	list, ok := raw.([]any)
	if !ok {
		return coerced(attribute, raw)
	}
	values := make([]any, len(list))
	for i, element := range list {
		values[i] = coerced(attribute, element)
	}
	return values
}

func coerced(attribute *core.Attribute, raw any) any {
	if typed, ok := attribute.Coerce(raw); ok {
		return typed
	}
	return nil
}
