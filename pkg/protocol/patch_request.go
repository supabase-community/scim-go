package protocol

import (
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/supabase-community/scim-go/internal/decode"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// PatchRequest is the body of a PATCH request, per RFC 7644, Section 3.5.2.
type PatchRequest struct {
	ID         string            `json:"-"`
	Version    string            `json:"-"`
	Schemas    []core.SchemaURI  `json:"schemas"`
	Operations []patch.Operation `json:"Operations"`
}

// DecodePatchRequest reads a PATCH body; RFC 7644 Section 3.5.2: it MUST contain the PatchOp "schemas" URI and one or more "Operations".
func DecodePatchRequest(body io.Reader) (*PatchRequest, error) {
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
	req, err := decode.JSON[*PatchRequest](raw)
	if err != nil {
		return nil, mistyped(err)
	}
	if !slices.ContainsFunc(req.Schemas, isPatchOp) {
		return nil, scimerrors.ErrInvalidSyntax(`"schemas" must contain ` + strconv.Quote(string(SchemaPatchOp)))
	}
	if len(req.Operations) == 0 {
		return nil, scimerrors.ErrInvalidSyntax(`"Operations" must contain at least one operation`)
	}
	return req, nil
}

// Patch applies the operations to document in place and returns the resource it describes; RFC 7644 Section 3.5.2: a PATCH request SHALL be treated as atomic.
func (r *PatchRequest) Patch[T any](document core.Object, schemas core.Schemas, opts ...patch.Option) (T, error) {
	if err := patch.Apply(document, r.Operations, schemas, opts...); err != nil {
		var zero T
		return zero, err
	}
	return fromDocument[T](document)
}

func mistyped(err error) error {
	if typeErr, ok := errors.AsType[*json.UnmarshalTypeError](err); ok {
		return scimerrors.ErrInvalidSyntax(strconv.Quote(typeErr.Field) + " has the wrong type: " + typeErr.Value)
	}
	return scimerrors.ErrInvalidSyntax(notJSON)
}

func isPatchOp(uri core.SchemaURI) bool {
	return strings.EqualFold(string(uri), string(SchemaPatchOp))
}
