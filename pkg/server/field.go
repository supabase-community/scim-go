package server

import "github.com/supabase-community/scim-go/pkg/core"

type Field[T any] struct {
	*core.Attribute
	accessor         Accessor[T]
	children         []*Field[T]
	elements         func(T) []any
	elementAccessors map[*core.Attribute]func(any) any
}

func NewField[T any](attribute *core.Attribute, accessor Accessor[T]) *Field[T] {
	return &Field[T]{Attribute: attribute, accessor: accessor}
}

func (f *Field[T]) With(children ...*Field[T]) *Field[T] {
	f.children = children
	f.SubAttributes = attributesOf(children)
	return f
}

// NewElements reads the elements of a multi-valued attribute, per RFC 7643, Section 2.4.
func NewElements[T, E any](attribute *core.Attribute, list func(T) []E, children ...*Field[E]) *Field[T] {
	attribute.SubAttributes = attributesOf(children)
	accessors := map[*core.Attribute]func(any) any{}
	for _, child := range children {
		accessors[child.Attribute] = func(element any) any { return child.accessor(element.(E)) }
	}
	return &Field[T]{
		Attribute:        attribute,
		elementAccessors: accessors,
		elements: func(item T) []any {
			var elements []any
			for _, element := range list(item) {
				elements = append(elements, element)
			}
			return elements
		},
	}
}

func NewMultiValued[T any](name string, list func(T) []core.Element, types ...string) *Field[T] {
	attribute := core.NewMultiValuedAttribute(name, types...)
	return NewElements(attribute, list,
		NewField(attribute.SubAttribute("value"), func(e core.Element) any { return e.Value }),
		NewField(attribute.SubAttribute("display"), func(e core.Element) any { return e.Display }),
		NewField(attribute.SubAttribute("type"), func(e core.Element) any { return e.Type }),
		NewField(attribute.SubAttribute("primary"), func(e core.Element) any { return e.Primary != nil && *e.Primary }),
		NewField(attribute.SubAttribute("$ref"), func(e core.Element) any { return e.Ref }),
	)
}

func attributesOf[T any](fields []*Field[T]) core.Attributes {
	attributes := make(core.Attributes, len(fields))
	for i, field := range fields {
		attributes[i] = field.Attribute
	}
	return attributes
}
