package protocol

import "encoding/json"

// PatchOperation is a single modification within a PATCH request, per RFC 7644, Section 3.5.2.
type PatchOperation struct {
	Op    PatchOp         `json:"op"`
	Path  string          `json:"path,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}
