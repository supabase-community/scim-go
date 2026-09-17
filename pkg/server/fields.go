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

func (fs Fields[T]) Accessors() Accessors[T] {
	accessors := Accessors[T]{}
	fs.collect(accessors)
	return accessors
}

func (fs Fields[T]) collect(accessors Accessors[T]) {
	for _, field := range fs {
		if field.accessor != nil {
			accessors[field.Attribute] = field.accessor
		}
		Fields[T](field.children).collect(accessors)
	}
}
