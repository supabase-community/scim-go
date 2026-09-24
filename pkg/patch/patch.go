package patch

import (
	"bytes"
	"encoding/json"
	"maps"

	"github.com/supabase-community/scim-go/pkg/core"
)

// Apply applies the operations to resource atomically, per RFC 7644, Section 3.5.2.
func Apply(resource core.Object, ops []Operation, schemas core.Schemas) error {
	working := clone(map[string]any(resource)).(map[string]any)
	if err := (&patcher{schemas: schemas}).run(working, ops); err != nil {
		return err
	}
	clear(resource)
	maps.Copy(resource, working)
	return nil
}

func decode[T any](raw []byte) (T, error) {
	var out T
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	err := decoder.Decode(&out)
	return out, err
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
