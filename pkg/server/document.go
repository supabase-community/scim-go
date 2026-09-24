package server

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type document map[string]any

func newDocument(item any) (document, error) {
	raw, err := json.Marshal(item)
	if err != nil {
		return nil, scimerrors.ErrInternal("could not encode the resource")
	}
	return protocol.Decode[document](bytes.NewReader(raw))
}

func asDocument(value any) document {
	switch v := value.(type) {
	case document:
		return v
	case map[string]any:
		return v
	}
	return nil
}

// RFC 7643 Section 2.1: attribute names are case insensitive.
func (d document) get(name string) any {
	if value, ok := d[name]; ok {
		return value
	}
	for key, value := range d {
		if strings.EqualFold(key, name) {
			return value
		}
	}
	return nil
}

type reader func(document) any

type readers struct {
	values   map[*core.Attribute]reader
	elements map[*core.Attribute]func(document) []any
}

func readersOf(schemas core.Schemas) readers {
	r := readers{values: map[*core.Attribute]reader{}, elements: map[*core.Attribute]func(document) []any{}}
	root := func(d document) document { return d }
	for _, name := range []string{"id", "externalId", "meta"} {
		attribute, _ := core.CommonAttribute(name)
		r.add(root, attribute)
	}
	r.add(root, schemas.Base().Attributes...)
	for _, extension := range schemas.Extensions() {
		r.add(func(d document) document { return asDocument(d.get(string(extension.ID))) }, extension.Attributes...)
	}
	return r
}

func (r readers) add(parent func(document) document, attributes ...*core.Attribute) {
	for _, attribute := range attributes {
		read := func(d document) any { return parent(d).get(attribute.Name) }
		r.values[attribute] = func(d document) any { return coerce(attribute, read(d)) }
		if attribute.MultiValued {
			r.elements[attribute] = func(d document) []any { list, _ := read(d).([]any); return list }
		}
		for _, sub := range attribute.SubAttributes {
			r.values[sub] = r.sub(attribute, sub, read)
		}
	}
}

// RFC 7644 Section 3.4.2.2: outside a value path, a sub-attribute holds the values of every element.
func (r readers) sub(parent, sub *core.Attribute, read reader) reader {
	if !parent.MultiValued {
		return func(d document) any { return coerce(sub, asDocument(read(d)).get(sub.Name)) }
	}
	elements := r.elements[parent]
	return func(d document) any {
		list := elements(d)
		values := make([]any, len(list))
		for i, element := range list {
			values[i] = asDocument(element).get(sub.Name)
		}
		return coerce(sub, values)
	}
}

func coerce(attribute *core.Attribute, value any) any {
	if attribute.Type == core.TypeComplex {
		return value
	}
	list, ok := value.([]any)
	if !ok {
		return coerced(attribute, value)
	}
	values := make([]any, len(list))
	for i, element := range list {
		values[i] = coerced(attribute, element)
	}
	return values
}

func coerced(attribute *core.Attribute, value any) any {
	if typed, ok := attribute.Coerce(value); ok {
		return typed
	}
	return nil
}
