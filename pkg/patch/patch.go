package patch

import (
	"github.com/supabase-community/scim-go/pkg/core"
)

// Apply returns a copy of resource with the operations applied atomically, per RFC 7644, Section 3.5.2; opts refine the caps enforced along the way, such as MaxFilterEvaluations.
func Apply(resource core.Object, ops []Operation, schemas core.Schemas, opts ...Option) (core.Object, error) {
	limits := defaultLimits()
	for _, opt := range opts {
		opt(&limits)
	}
	working := core.Object(clone(map[string]any(resource)).(map[string]any))
	if err := (&patcher{schemas: schemas, budget: limits.budget(), indexes: make(indexes)}).run(working, ops); err != nil {
		return nil, err
	}
	return working, nil
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
