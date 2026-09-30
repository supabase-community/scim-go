package patch

import (
	"github.com/supabase-community/scim-go/pkg/core"
)

// Apply applies the operations to resource in place; RFC 7644 Section 3.5.2 atomicity is left to the caller.
func Apply(resource core.Object, ops []Operation, schemas core.Schemas, opts ...Option) error {
	limits := defaultLimits()
	for _, opt := range opts {
		opt(&limits)
	}
	return (&patcher{schemas: schemas, budget: limits.budget(), indexes: indexCache{}}).run(resource, ops)
}
