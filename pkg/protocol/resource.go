package protocol

import (
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"

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

// RFC 7644 Section 3.3: the request body MUST be a JSON object.
func readDocument(r io.Reader) (map[string]any, error) {
	document, err := Decode[map[string]any](r)
	if err != nil {
		return nil, err
	}
	if document == nil {
		return nil, scimerrors.ErrInvalidSyntax("request body is not a JSON object")
	}
	if !wellFormedNames(document) {
		return nil, scimerrors.ErrInvalidSyntax("request body has an attribute name that is not US-ASCII or repeats in another case")
	}
	return document, nil
}

// RFC 7643 Section 2.1: attribute names are case insensitive and the character set is US-ASCII.
func wellFormedNames(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		seen := make(map[string]struct{}, len(v))
		for name, element := range v {
			folded := strings.ToLower(name)
			if _, repeated := seen[folded]; repeated || !ascii(name) || !wellFormedNames(element) {
				return false
			}
			seen[folded] = struct{}{}
		}
	case []any:
		for _, element := range v {
			if !wellFormedNames(element) {
				return false
			}
		}
	}
	return true
}

func ascii(name string) bool {
	for i := 0; i < len(name); i++ {
		if name[i] >= utf8.RuneSelf {
			return false
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
