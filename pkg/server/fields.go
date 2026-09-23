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
	return attributesOf(fs)
}

func (fs Fields[T]) accessors() accessorSet[T] {
	accessors := accessorSet[T]{}
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

func (fs Fields[T]) elementAccessors() map[*core.Attribute]func(any) any {
	accessors := map[*core.Attribute]func(any) any{}
	fs.walk(func(field *Field[T]) {
		maps.Copy(accessors, field.elementAccessors)
	})
	return accessors
}

func (fs Fields[T]) elements() map[*core.Attribute]func(T) []any {
	elements := map[*core.Attribute]func(T) []any{}
	fs.walk(func(field *Field[T]) {
		if field.elements != nil {
			elements[field.Attribute] = field.elements
		}
	})
	return elements
}

// RFC 7644 Section 3.10: the attribute notation of every attribute that has an accessor.
func (fs Fields[T]) paths() map[*core.Attribute]string {
	paths := map[*core.Attribute]string{}
	for _, field := range fs {
		paths[field.Attribute] = field.Name
		for _, child := range field.children {
			paths[child.Attribute] = field.Name + "." + child.Name
		}
		for attribute := range field.elementAccessors {
			paths[attribute] = field.Name + "." + attribute.Name
		}
	}
	return paths
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
