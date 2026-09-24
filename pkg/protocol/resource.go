package protocol

import (
	"io"

	"github.com/supabase-community/scim-go/pkg/core"
)

// DecodeResource decodes a resource body; RFC 7644 Sections 3.3 and 3.5.1: readOnly attribute values SHALL be ignored.
func DecodeResource[T any](body io.Reader, existing any, schemas core.Schemas) (T, error) {
	var item T
	document, err := readDocument(body)
	if err != nil {
		return item, err
	}
	prior, err := toDocument(existing)
	if err != nil {
		return item, err
	}
	return fromDocument[T](writable(document, prior, schemas))
}

func writable(document, existing core.Object, schemas core.Schemas) core.Object {
	if len(schemas) == 0 {
		return document
	}
	base := func(name string) *core.Attribute {
		attribute, _ := schemas.Resolve("", name, "")
		return attribute
	}
	out := core.Object(writableObject(base, document, existing))
	for _, extension := range schemas.Extensions() {
		uri := string(extension.ID)
		raw := out.Get(uri)
		out.Remove(uri)
		body, isObject := raw.(map[string]any)
		previous, _ := existing.Get(uri).(map[string]any)
		if object := writableObject(extension.Attributes.Lookup, body, previous); len(object) > 0 {
			out[uri] = object
		} else if raw != nil && !isObject {
			out[uri] = raw
		}
	}
	return out
}

func writableObject(lookup func(string) *core.Attribute, body, existing map[string]any) map[string]any {
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
		return writableObject(attribute.SubAttribute, v, previous)
	case []any:
		elements := make([]any, len(v))
		for i, element := range v {
			if object, ok := element.(map[string]any); ok {
				element = writableObject(attribute.SubAttribute, object, nil)
			}
			elements[i] = element
		}
		return elements
	}
	return value
}
