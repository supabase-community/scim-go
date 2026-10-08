package protocol

import (
	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
)

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
		if writableObject(extension.Attributes.Lookup, body, previous); !value.AllUnassigned(body) {
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
		return writableValue(attribute, value, core.Object(existing).Get(key)), true
	})
	// RFC 7644 Section 3.5.1: readOnly values SHALL be ignored.
	for key, value := range existing {
		switch attribute := lookup(key); {
		case attribute == nil:
			continue
		case attribute.Mutability == core.MutabilityReadOnly:
			body[key] = value
		case attribute.Mutability == core.MutabilityImmutable && !attribute.Required:
			if !core.Object(body).Has(key) {
				body[key] = value
			}
		}
	}
}

func writableValue(attribute *core.Attribute, candidate, existing any) any {
	if len(attribute.SubAttributes) == 0 {
		return candidate
	}
	switch v := candidate.(type) {
	case map[string]any:
		previous, _ := existing.(map[string]any)
		writableObject(attribute.SubAttribute, v, previous)
	case []any:
		stored := value.ByIdentity(attribute, existing)
		for _, element := range v {
			if object, ok := element.(map[string]any); ok {
				writableObject(attribute.SubAttribute, object, stored[value.Identity(attribute, object)])
			}
		}
	}
	return candidate
}
