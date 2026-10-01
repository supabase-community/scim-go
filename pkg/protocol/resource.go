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
	document, err := readDocument(body)
	if err != nil {
		var item T
		return item, err
	}
	return resourceFrom[T](document, existing, schemas)
}

func resourceFrom[T any](document core.Object, existing any, schemas core.Schemas) (T, error) {
	var prior core.Object
	if existing != nil {
		var err error
		if prior, err = core.NewObject(existing); err != nil {
			var item T
			return item, err
		}
	}
	return fromDocument[T](writable(document, prior, schemas))
}

// RFC 7644 Section 3.1: a SCIM resource is a JSON object.
func readDocument(r io.Reader) (map[string]any, error) {
	document, err := Decode[map[string]any](r)
	if err != nil {
		return nil, err
	}
	if document == nil {
		return nil, scimerrors.ErrInvalidSyntax("request body is not a JSON object")
	}
	if err := checkNames(document); err != nil {
		return nil, err
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
