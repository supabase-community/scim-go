package patch

import "github.com/supabase-community/scim-go/pkg/core"

// Apply applies the operations to resource atomically, per RFC 7644, Section 3.5.2.
func Apply(resource any, ops []Operation, schemas []*core.Schema) error {
	return newEngine(schemas).apply(resource, ops)
}
