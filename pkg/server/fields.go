package server

import "github.com/supabase-community/scim-go/pkg/core"

type Fields[T Entity] []*Field[T]

func (fs Fields[T]) Attributes() core.Attributes {
	attributes := make(core.Attributes, len(fs))
	for i, field := range fs {
		attributes[i] = field.Attribute
	}
	return attributes
}

func (fs Fields[T]) Getters() Getters[T] {
	getters := Getters[T]{}
	fs.collect(getters)
	return getters
}

func (fs Fields[T]) collect(getters Getters[T]) {
	for _, field := range fs {
		if field.get != nil {
			getters[field.Attribute] = field.get
		}
		Fields[T](field.children).collect(getters)
	}
}
