package core

import "strings"

type Attributes []*Attribute

func (attrs Attributes) Lookup(name string) *Attribute {
	for _, attribute := range attrs {
		if strings.EqualFold(attribute.Name, name) {
			return attribute
		}
	}
	return nil
}
