package protocol

import (
	"encoding/json"

	"github.com/supabase-community/scim-go/pkg/core"
)

// PatchOp is the kind of modification in a PATCH operation, per RFC 7644, Section 3.5.2.
type PatchOp string

const (
	PatchOpAdd     PatchOp = "add"
	PatchOpRemove  PatchOp = "remove"
	PatchOpReplace PatchOp = "replace"
)

// PatchOperation is a single modification within a PATCH request, per RFC 7644, Section 3.5.2.
type PatchOperation struct {
	Op    PatchOp         `json:"op"`
	Path  string          `json:"path,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}

// PatchRequest is the body of a PATCH request, per RFC 7644, Section 3.5.2.
type PatchRequest struct {
	Schemas    []core.SchemaURI `json:"schemas"`
	Operations []PatchOperation `json:"Operations"`
}
