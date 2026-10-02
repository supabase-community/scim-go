package protocol

import (
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/supabase-community/scim-go/internal/decode"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// DecodeDocument reads a resource body; RFC 7644 Section 3.1: a SCIM resource is a JSON object.
func DecodeDocument(body io.Reader) (core.Object, error) {
	raw, err := read(body)
	if err != nil {
		return nil, err
	}
	document, err := objectOf(raw)
	if err != nil {
		return nil, err
	}
	if err := checkNames(document); err != nil {
		return nil, err
	}
	return document, nil
}

// ResourceFrom drops readOnly values from document in place and builds a resource from it; RFC 7644 Sections 3.3 and 3.5.1: readOnly attribute values SHALL be ignored.
func ResourceFrom[T any](document, existing core.Object, schemas core.Schemas) (T, error) {
	return fromDocument[T](writable(document, existing, schemas))
}

func objectOf(raw []byte) (map[string]any, error) {
	value, err := decode.Value(raw)
	if value == nil && err == nil {
		return nil, scimerrors.ErrInvalidSyntax("request body is not a JSON object")
	}
	document, ok := value.(map[string]any)
	if !ok {
		return nil, scimerrors.ErrInvalidSyntax(notJSON)
	}
	return document, nil
}

// RFC 7643 Section 2.1: attribute names are case insensitive and the character set is US-ASCII.
func checkNames(document any) error {
	if !wellFormedNames(document) {
		return scimerrors.ErrInvalidSyntax("request body has an attribute name that is not US-ASCII or repeats in another case")
	}
	return nil
}

func wellFormedNames(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		return wellFormedFields(v)
	case []any:
		return wellFormedElements(v)
	}
	return true
}

func wellFormedFields(fields map[string]any) bool {
	seen := make(map[string]struct{}, len(fields))
	for name, element := range fields {
		folded := strings.ToLower(name)
		if _, repeated := seen[folded]; repeated || !ascii(name) || !wellFormedNames(element) {
			return false
		}
		seen[folded] = struct{}{}
	}
	return true
}

func wellFormedElements(elements []any) bool {
	for _, element := range elements {
		if !wellFormedNames(element) {
			return false
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
