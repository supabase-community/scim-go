package patch

import (
	"bytes"
	"encoding/json"
)

// Operation is a single modification within a PATCH request, per RFC 7644, Section 3.5.2.
type Operation struct {
	Op    Op              `json:"op"`
	Path  string          `json:"path,omitempty"`
	Value json.RawMessage `json:"value,omitempty"`
}

// RFC 7644 Section 3.5.2.2: "remove" selects its target by "path" alone and defines no "value".
func (o Operation) hasValue() bool {
	return len(o.Value) > 0 && !bytes.Equal(bytes.TrimSpace(o.Value), []byte("null"))
}
