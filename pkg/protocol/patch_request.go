package protocol

import (
	"encoding/json"
	"io"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// PatchRequest is the body of a PATCH request, per RFC 7644, Section 3.5.2.
type PatchRequest struct {
	Schemas    []core.SchemaURI  `json:"schemas"`
	Operations []patch.Operation `json:"Operations"`
}

// DecodePatchRequest reads a PATCH request body, per RFC 7644 Section 3.5.2: it MUST be a JSON object.
func DecodePatchRequest(body io.Reader) (*PatchRequest, error) {
	req, err := Decode[*PatchRequest](body)
	if err != nil {
		return nil, err
	}
	if req == nil {
		return nil, scimerrors.ErrInvalidSyntax("request body is not valid JSON")
	}
	return req, nil
}

// Apply applies the operations to resource atomically, per RFC 7644, Section 3.5.2.
func (r *PatchRequest) Apply(resource any, schemas []*core.Schema) error {
	return patch.Apply(resource, r.Operations, schemas)
}

// Patch returns a new resource; resource is left unchanged, per RFC 7644 Section 3.5.2.
func (r *PatchRequest) Patch[T any](resource T, schemas []*core.Schema) (T, error) {
	var zero T
	existing, err := toDocument(resource)
	if err != nil {
		return zero, err
	}
	document, err := toDocument(resource)
	if err != nil {
		return zero, err
	}
	if err := r.Apply(document, schemas); err != nil {
		return zero, err
	}
	return fromDocument[T](writable(document, existing, schemas))
}

func Decode[T any](body io.Reader) (T, error) {
	var req T
	decoder := json.NewDecoder(body)
	decoder.UseNumber()
	if err := decoder.Decode(&req); err != nil {
		return req, scimerrors.ErrInvalidSyntax("request body is not valid JSON")
	}
	return req, nil
}
