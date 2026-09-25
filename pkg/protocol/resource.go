package protocol

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// DecodeResource decodes a resource body; RFC 7644 Sections 3.3 and 3.5.1: readOnly attribute values SHALL be ignored.
func DecodeResource[T any](body io.Reader, existing any, schemas core.Schemas) (T, error) {
	var item T
	document, err := readDocument(body)
	if err != nil {
		return item, err
	}
	var prior core.Object
	if existing != nil {
		if prior, err = core.NewObject(existing); err != nil {
			return item, err
		}
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

// RFC 7644 Section 3.3: the request body MUST be a JSON object.
func readDocument(r io.Reader) (map[string]any, error) {
	document, err := Decode[map[string]any](r)
	if err != nil {
		return nil, err
	}
	if document == nil {
		return nil, scimerrors.ErrInvalidSyntax("request body is not a JSON object")
	}
	if !distinctNames(document) {
		return nil, scimerrors.ErrInvalidSyntax("request body repeats an attribute name")
	}
	return document, nil
}

// RFC 7643 Section 2.1: attribute names are case insensitive.
func distinctNames(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		seen := make(map[string]struct{}, len(v))
		for name, element := range v {
			folded := strings.ToLower(name)
			if _, repeated := seen[folded]; repeated || !distinctNames(element) {
				return false
			}
			seen[folded] = struct{}{}
		}
	case []any:
		for _, element := range v {
			if !distinctNames(element) {
				return false
			}
		}
	}
	return true
}

func fromDocument[T any](document map[string]any) (T, error) {
	var item T
	raw, err := json.Marshal(document)
	if err != nil {
		return item, scimerrors.ErrInternal("could not encode the request body")
	}
	if err := json.Unmarshal(raw, &item); err != nil {
		return item, scimerrors.ErrInvalidSyntax("request body does not match the resource")
	}
	return item, nil
}
