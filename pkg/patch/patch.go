package patch

import (
	"github.com/supabase-community/scim-go/pkg/core"
)

// Apply returns a copy of resource with the operations applied atomically, per RFC 7644, Section 3.5.2.
func Apply(resource core.Object, ops []Operation, schemas core.Schemas) (core.Object, error) {
	return ApplyWithin(resource, ops, schemas, 0)
}

// ApplyWithin is Apply that refuses the request before its value filters would check more than maxEvaluations clauses; zero lifts the cap.
func ApplyWithin(resource core.Object, ops []Operation, schemas core.Schemas, maxEvaluations int) (core.Object, error) {
	working := core.Object(clone(map[string]any(resource)).(map[string]any))
	if err := (&patcher{schemas: schemas, budget: &budget{max: maxEvaluations}}).run(working, ops); err != nil {
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
