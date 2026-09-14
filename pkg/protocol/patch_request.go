package protocol

import (
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
)

// PatchRequest is the body of a PATCH request, per RFC 7644, Section 3.5.2.
type PatchRequest struct {
	Schemas    []core.SchemaURI  `json:"schemas"`
	Operations []patch.Operation `json:"Operations"`
}

// Apply applies the operations to resource atomically, per RFC 7644, Section 3.5.2.
func (r *PatchRequest) Apply(resource any, schemas []*core.Schema) error {
	return patch.Apply(resource, r.Operations, schemas)
}
