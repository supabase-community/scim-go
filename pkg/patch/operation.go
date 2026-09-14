package patch

import "encoding/json"

// Operation is a single modification within a PATCH request, per RFC 7644, Section 3.5.2.
type Operation struct {
	Op    Op              `json:"op"`
	Path  string          `json:"path,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}
