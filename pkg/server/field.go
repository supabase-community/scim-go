package server

import "github.com/supabase-community/scim-go/pkg/core"

type Field[T Entity] struct {
	*core.Attribute
	accessor         Accessor[T]
	children         []*Field[T]
	elements         func(T) []any
	elementAccessors map[*core.Attribute]func(any) any
}

func NewField[T Entity](attribute *core.Attribute, accessor Accessor[T]) *Field[T] {
	return &Field[T]{
		Attribute: attribute,
		accessor:  accessor,
	}
}

func (f *Field[T]) With(children ...*Field[T]) *Field[T] {
	f.children = children
	subAttributes := make(core.Attributes, len(children))
	for i, child := range children {
		subAttributes[i] = child.Attribute
	}
	f.SubAttributes = subAttributes
	return f
}

type ElementField[E any] struct {
	*core.Attribute
	accessor func(E) any
}

func NewElementField[E any](attribute *core.Attribute, accessor func(E) any) *ElementField[E] {
	return &ElementField[E]{
		Attribute: attribute,
		accessor:  accessor,
	}
}

func NewElements[T Entity, E any](attribute *core.Attribute, list func(T) []E, children ...*ElementField[E]) *Field[T] {
	subAttributes := make(core.Attributes, len(children))
	elementAccessors := make(map[*core.Attribute]func(any) any, len(children))
	for i, child := range children {
		subAttributes[i] = child.Attribute
		elementAccessors[child.Attribute] = func(element any) any { return child.accessor(element.(E)) }
	}
	attribute.SubAttributes = subAttributes
	return &Field[T]{
		Attribute: attribute,
		elements: func(item T) []any {
			items := list(item)
			elements := make([]any, len(items))
			for i, element := range items {
				elements[i] = element
			}
			return elements
		},
		elementAccessors: elementAccessors,
	}
}

func NewMultiValued[T Entity](name string, list func(T) []core.Element, types ...string) *Field[T] {
	attribute := core.NewMultiValuedAttribute(name, types...)
	return NewElements(attribute, list,
		NewElementField(attribute.SubAttribute("value"), func(e core.Element) any { return e.Value }),
		NewElementField(attribute.SubAttribute("display"), func(e core.Element) any { return e.Display }),
		NewElementField(attribute.SubAttribute("type"), func(e core.Element) any { return e.Type }),
		NewElementField(attribute.SubAttribute("primary"), func(e core.Element) any { return e.Primary != nil && *e.Primary }),
		NewElementField(attribute.SubAttribute("$ref"), func(e core.Element) any { return e.Ref }),
	)
}
