package protocol

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

func toDocument(resource any) (map[string]any, error) {
	raw, err := json.Marshal(resource)
	if err != nil {
		return nil, scimerrors.ErrInternal("could not encode the resource")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	document := map[string]any{}
	if err := decoder.Decode(&document); err != nil {
		return nil, scimerrors.ErrInternal("could not decode the resource")
	}
	return document, nil
}

// RFC 7644 Section 3.3: the request body MUST be a JSON object.
func readDocument(r io.Reader) (map[string]any, error) {
	var document map[string]any
	decoder := json.NewDecoder(r)
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil || document == nil {
		return nil, scimerrors.ErrInvalidSyntax("request body is not a JSON object")
	}
	return document, nil
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
