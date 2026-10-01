package protocol

import (
	"io"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
)

// ReplaceRequest is the body of a PUT request, per RFC 7644, Section 3.5.1.
type ReplaceRequest struct {
	Attributes core.Object
}

// DecodeReplaceRequest reads a PUT body; RFC 7644 Section 3.1: a SCIM resource is a JSON object.
func DecodeReplaceRequest(body io.Reader) (*ReplaceRequest, error) {
	document, err := readDocument(body)
	if err != nil {
		return nil, err
	}
	return &ReplaceRequest{Attributes: document}, nil
}

// Replace returns the replacement for existing; RFC 7644 Section 3.5.1: readOnly values SHALL be ignored.
func (r *ReplaceRequest) Replace[T any](existing T, schemas core.Schemas) (T, error) {
	attributes, _ := value.Clone(map[string]any(r.Attributes)).(map[string]any)
	return resourceFrom[T](attributes, existing, schemas)
}
