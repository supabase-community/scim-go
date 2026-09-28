package core

import "strings"

type Attributes []*Attribute

func (attrs Attributes) Lookup(name string) *Attribute {
	return lookupFold(attrs, name, func(attribute *Attribute) string { return attribute.Name })
}

func lookupFold[T any](items []T, target string, key func(T) string) T {
	var zero T
	for _, item := range items {
		if strings.EqualFold(key(item), target) {
			return item
		}
	}
	return zero
}
