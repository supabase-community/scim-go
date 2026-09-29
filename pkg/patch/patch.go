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
	return (&patcher{schemas: schemas, budget: limits.budget()}).run(resource, ops)
}

func clone(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, element := range typed {
			cloned[key] = clone(element)
		}
		return cloned
	case []any:
		cloned := make([]any, len(typed))
		for i, element := range typed {
			cloned[i] = clone(element)
		}
		return cloned
	default:
		return typed
	}
}
