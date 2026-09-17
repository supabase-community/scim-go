package server

import "github.com/supabase-community/scim-go/pkg/core"

type Field[T Entity] struct {
	*core.Attribute
	get      Getter[T]
	children []*Field[T]
}

func NewField[T Entity](attribute *core.Attribute, get Getter[T]) *Field[T] {
	return &Field[T]{Attribute: attribute, get: get}
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
