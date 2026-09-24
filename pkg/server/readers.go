package server

import (
	"iter"

	"github.com/supabase-community/scim-go/pkg/core"
)

func asObject(value any) core.Object {
	switch v := value.(type) {
	case core.Object:
		return v
	case map[string]any:
		return v
	}
	return nil
}

type reader func(core.Object) any

type readers struct {
	values    map[*core.Attribute]reader
	elements  map[*core.Attribute]func(core.Object) []any
	order     []*core.Attribute
	immutable []*core.Attribute
}

func readersOf(schemas core.Schemas) readers {
	r := &readers{values: map[*core.Attribute]reader{}, elements: map[*core.Attribute]func(core.Object) []any{}}
	root := func(d core.Object) core.Object { return d }
	for _, name := range []string{"id", "externalId", "meta"} {
		attribute, _ := core.CommonAttribute(name)
		r.add(root, attribute)
	}
	r.add(root, schemas.Base().Attributes...)
	for _, extension := range schemas.Extensions() {
		r.add(func(d core.Object) core.Object { return asObject(d.Get(string(extension.ID))) }, extension.Attributes...)
	}
	return *r
}

func (r *readers) add(parent func(core.Object) core.Object, attributes ...*core.Attribute) {
	for _, attribute := range attributes {
		read := func(d core.Object) any { return parent(d).Get(attribute.Name) }
		r.set(attribute, func(d core.Object) any { return coerce(attribute, read(d)) })
		r.track(attribute)
		if attribute.MultiValued {
			r.elements[attribute] = func(d core.Object) []any { list, _ := read(d).([]any); return list }
		}
		for _, sub := range attribute.SubAttributes {
			r.set(sub, r.sub(attribute, sub, read))
			if !attribute.MultiValued {
				r.track(sub)
			}
		}
	}
}

func (r readers) all() iter.Seq2[*core.Attribute, reader] {
	return func(yield func(*core.Attribute, reader) bool) {
		for _, attribute := range r.order {
			if !yield(attribute, r.values[attribute]) {
				return
			}
		}
	}
}

func (r *readers) set(attribute *core.Attribute, read reader) {
	r.values[attribute] = read
	r.order = append(r.order, attribute)
}

// RFC 7644 Section 3.4.2.2: outside a value path, a sub-attribute holds the values of every element.
func (r readers) sub(parent, sub *core.Attribute, read reader) reader {
	if !parent.MultiValued {
		return func(d core.Object) any { return coerce(sub, asObject(read(d)).Get(sub.Name)) }
	}
	elements := r.elements[parent]
	return func(d core.Object) any {
		list := elements(d)
		values := make([]any, len(list))
		for i, element := range list {
			values[i] = asObject(element).Get(sub.Name)
		}
		return coerce(sub, values)
	}
}

func (r *readers) track(attribute *core.Attribute) {
	if attribute.Mutability == core.MutabilityImmutable {
		r.immutable = append(r.immutable, attribute)
	}
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
