package protocol

import "github.com/supabase-community/scim-go/pkg/core"

// RFC 7644 Sections 3.3 and 3.5.1: values provided for readOnly attributes SHALL be ignored.
func Writable(document map[string]any, existing any, schemas []*core.Schema) (map[string]any, error) {
	if len(schemas) == 0 {
		return document, nil
	}
	prior, err := toDocument(existing)
	if err != nil {
		return nil, err
	}
	base := func(name string) *core.Attribute {
		attribute, _ := schemas[0].Resolve(name)
		return attribute
	}
	out := writable(base, document, prior)
	for _, extension := range schemas[1:] {
		uri := string(extension.ID)
		body, _ := out[uri].(map[string]any)
		previous, _ := prior[uri].(map[string]any)
		if object := writable(extension.Attributes.Lookup, body, previous); len(object) > 0 {
			out[uri] = object
		}
	}
	return out, nil
}

func writable(lookup func(string) *core.Attribute, body, existing map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range body {
		attribute := lookup(key)
		switch {
		case attribute == nil:
			out[key] = value
		case attribute.Mutability != core.MutabilityReadOnly:
			out[key] = writableValue(attribute, value, existing[key])
		}
	}
	for key, value := range existing {
		if attribute := lookup(key); attribute != nil && attribute.Mutability == core.MutabilityReadOnly {
			out[key] = value
		}
	}
	return out
}

func writableValue(attribute *core.Attribute, value, existing any) any {
	if len(attribute.SubAttributes) == 0 {
		return value
	}
	switch v := value.(type) {
	case map[string]any:
		previous, _ := existing.(map[string]any)
		return writable(attribute.SubAttribute, v, previous)
	case []any:
		elements := make([]any, len(v))
		for i, element := range v {
			if object, ok := element.(map[string]any); ok {
				element = writable(attribute.SubAttribute, object, nil)
			}
			elements[i] = element
		}
		return elements
	}
	return value
}
