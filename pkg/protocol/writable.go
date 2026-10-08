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
		body := value.AsObject(raw)
		isObject := body != nil
		if !isObject {
			body = core.Object{}
		}
		if writableObject(extension.Attributes.Lookup, body, value.AsObject(existing.Get(uri))); !value.AllUnassigned(body) {
			document[uri] = map[string]any(body)
		} else if raw != nil && !isObject {
			document[uri] = raw
		}
	}
	return document
}

func writableObject(lookup func(string) *core.Attribute, body, existing core.Object) {
	prune(body, func(key string, item any) (any, bool) {
		attribute := lookup(key)
		switch {
		case attribute == nil:
			return item, true
		case attribute.Mutability == core.MutabilityReadOnly:
			return nil, false
		}
		return writableValue(attribute, item, existing.Get(key)), true
	})
	// RFC 7644 Section 3.5.1: readOnly values SHALL be ignored.
	for key, item := range existing {
		switch attribute := lookup(key); {
		case attribute == nil:
			continue
		case attribute.Mutability == core.MutabilityReadOnly:
			body[key] = item
		case attribute.Mutability == core.MutabilityImmutable && !attribute.Required:
			if !body.Has(key) {
				body[key] = item
			}
		}
	}
}

func writableValue(attribute *core.Attribute, candidate, existing any) any {
	if len(attribute.SubAttributes) == 0 {
		return candidate
	}
	if object := value.AsObject(candidate); object != nil {
		writableObject(attribute.SubAttribute, object, value.AsObject(existing))
		return candidate
	}
	if list, ok := candidate.([]any); ok {
		elements, _ := existing.([]any)
		stored := value.NewSet(attribute, elements)
		for _, element := range list {
			if object := value.AsObject(element); object != nil {
				writableObject(attribute.SubAttribute, object, stored.Find(object))
			}
		}
	}
	return candidate
}
