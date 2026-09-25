package protocol

import "github.com/supabase-community/scim-go/pkg/core"

func writable(document, existing core.Object, schemas core.Schemas) core.Object {
	if len(schemas) == 0 {
		return document
	}
	base := func(name string) *core.Attribute {
		attribute, _ := schemas.Resolve("", name, "")
		return attribute
	}
	writableObject(base, document, existing)
	for _, extension := range schemas.Extensions() {
		uri := string(extension.ID)
		raw := document.Get(uri)
		document.Remove(uri)
		body, isObject := raw.(map[string]any)
		if !isObject {
			body = map[string]any{}
		}
		previous, _ := existing.Get(uri).(map[string]any)
		if writableObject(extension.Attributes.Lookup, body, previous); len(body) > 0 {
			document[uri] = body
		} else if raw != nil && !isObject {
			document[uri] = raw
		}
	}
	return document
}

func writableObject(lookup func(string) *core.Attribute, body, existing map[string]any) {
	prune(body, func(key string, value any) (any, bool) {
		attribute := lookup(key)
		switch {
		case attribute == nil:
			return value, true
		case attribute.Mutability == core.MutabilityReadOnly:
			return nil, false
		}
		return writableValue(attribute, value, existing[key]), true
	})
	for key, value := range existing {
		if attribute := lookup(key); attribute != nil && attribute.Mutability == core.MutabilityReadOnly {
			body[key] = value
		}
	}
}

func writableValue(attribute *core.Attribute, value, existing any) any {
	if len(attribute.SubAttributes) == 0 {
		return value
	}
	switch v := value.(type) {
	case map[string]any:
		previous, _ := existing.(map[string]any)
		writableObject(attribute.SubAttribute, v, previous)
	case []any:
		for _, element := range v {
			if object, ok := element.(map[string]any); ok {
				writableObject(attribute.SubAttribute, object, nil)
			}
		}
	}
	return value
}
