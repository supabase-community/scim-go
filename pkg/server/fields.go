package server

import (
	"maps"

	"github.com/supabase-community/scim-go/pkg/core"
)

type Fields[T Entity] []*Field[T]

func NewFields[T Entity](fields ...*Field[T]) Fields[T] {
	return fields
}

func (fs Fields[T]) Attributes() core.Attributes {
	attributes := make(core.Attributes, len(fs))
	for i, field := range fs {
		attributes[i] = field.Attribute
	}
	return attributes
}

func (fs Fields[T]) Accessors() Accessors[T] {
	accessors := Accessors[T]{}
	fs.walk(func(field *Field[T]) {
		if field.accessor != nil {
			accessors[field.Attribute] = field.accessor
		}
		for attribute, accessor := range field.elementAccessors {
			accessors[attribute] = flatten(field.elements, accessor)
		}
	})
	return accessors
}

func (fs Fields[T]) ElementAccessors() map[*core.Attribute]func(any) any {
	accessors := map[*core.Attribute]func(any) any{}
	fs.walk(func(field *Field[T]) {
		maps.Copy(accessors, field.elementAccessors)
	})
	return accessors
}

func (fs Fields[T]) Elements() map[*core.Attribute]func(T) []any {
	elements := map[*core.Attribute]func(T) []any{}
	fs.walk(func(field *Field[T]) {
		if field.elements != nil {
			elements[field.Attribute] = field.elements
		}
	})
	return elements
}

func (fs Fields[T]) walk(visit func(*Field[T])) {
	for _, field := range fs {
		visit(field)
		Fields[T](field.children).walk(visit)
	}
}

// RFC 7644 3.4.2.2 - outside a value path, a sub-attribute holds the values of every element.
func flatten[T Entity](elements func(T) []any, accessor func(any) any) Accessor[T] {
	return func(item T) any {
		list := elements(item)
		values := make([]any, len(list))
		for i, element := range list {
			values[i] = accessor(element)
		}
		return values
	}
}
