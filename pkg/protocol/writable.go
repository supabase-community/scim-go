package protocol

import (
	"encoding/json"

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
		return writableValue(attribute, value, core.Object(existing).Get(key)), true
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
		stored := byIdentity(attribute, existing)
		for _, element := range v {
			if object, ok := element.(map[string]any); ok {
				writableObject(attribute.SubAttribute, object, stored[identity(attribute, object)])
			}
		}
	}
	return value
}

func byIdentity(attribute *core.Attribute, existing any) map[string]map[string]any {
	elements, _ := existing.([]any)
	stored := make(map[string]map[string]any, len(elements))
	for _, element := range elements {
		object, ok := element.(map[string]any)
		if id := identity(attribute, object); ok && id != "" && stored[id] == nil {
			stored[id] = object
		}
	}
	return stored
}

// RFC 7643 Section 2.4: "value" identifies an element; without one, the client-visible writable sub-attributes do.
func identity(attribute *core.Attribute, element core.Object) string {
	if sub := attribute.SubAttribute("value"); sub != nil {
		key, _ := value.Key(element)
		folded, _ := value.Fold(sub, key).(string)
		return folded
	}
	folded := []any{}
	for _, sub := range attribute.SubAttributes {
		if sub.Mutability != core.MutabilityReadOnly && !value.Hidden(nil, sub) {
			folded = append(folded, value.Fold(sub, element.Get(sub.Name)))
		}
	}
	if len(folded) == 0 {
		return ""
	}
	raw, _ := json.Marshal(folded)
	return string(raw)
}
