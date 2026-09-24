package protocol

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

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
	if !slices.ContainsFunc(req.Schemas, isPatchOp) {
		return nil, scimerrors.ErrInvalidSyntax(`"schemas" must contain ` + strconv.Quote(string(SchemaPatchOp)))
	}
	if len(req.Operations) == 0 {
		return nil, scimerrors.ErrInvalidSyntax(`"Operations" must contain at least one operation`)
	}
	return req, nil
}

func isPatchOp(uri core.SchemaURI) bool {
	return strings.EqualFold(string(uri), string(SchemaPatchOp))
}

// Apply returns a copy of resource with the operations applied atomically, per RFC 7644, Section 3.5.2.
func (r *PatchRequest) Apply(resource core.Object, schemas core.Schemas) (core.Object, error) {
	return patch.Apply(resource, r.Operations, schemas)
}

// Patch returns a new resource; resource is left unchanged, per RFC 7644 Section 3.5.2.
func (r *PatchRequest) Patch[T any](resource T, schemas core.Schemas) (T, error) {
	var zero T
	existing, err := core.NewObject(resource)
	if err != nil {
		return zero, err
	}
	patched, err := r.Apply(existing, schemas)
	if err != nil {
		return zero, err
	}
	return fromDocument[T](writable(patched, existing, schemas))
}

func Decode[T any](body io.Reader) (T, error) {
	var req T
	decoder := json.NewDecoder(body)
	decoder.UseNumber()
	if err := decoder.Decode(&req); err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return req, scimerrors.ErrTooLarge("request body is too large")
		}
		return req, scimerrors.ErrInvalidSyntax("request body is not valid JSON")
	}
	return req, nil
}
