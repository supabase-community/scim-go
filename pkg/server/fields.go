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

// readers holds the accessors the repository and the filter/sort evaluator both need.
type readers[T Entity] struct {
	accessors        accessorSet[T]
	elementAccessors map[*core.Attribute]func(any) any
	elements         map[*core.Attribute]func(T) []any
}

func (fs Fields[T]) readers() readers[T] {
	return readers[T]{
		accessors:        withCommonAccessors(fs.accessors()),
		elementAccessors: fs.elementAccessors(),
		elements:         fs.elements(),
	}
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
