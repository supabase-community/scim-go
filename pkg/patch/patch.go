package patch

import (
	"bytes"
	"encoding/json"
	"maps"
	"reflect"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// Apply applies the operations to resource atomically, per RFC 7644, Section 3.5.2.
func Apply(resource any, ops []Operation, schemas []*core.Schema) error {
	p := &patcher{schemas: schemas}

	if m, ok := resource.(map[string]any); ok {
		working := clone(m).(map[string]any)
		if err := p.run(working, ops); err != nil {
			return err
		}
		clear(m)
		maps.Copy(m, working)
		return nil
	}

	raw, err := json.Marshal(resource)
	if err != nil {
		return scimerrors.ErrInvalidSyntax("resource cannot be encoded")
	}
	working, err := decode[map[string]any](raw)
	if err != nil || working == nil {
		return scimerrors.ErrInvalidSyntax("resource is not a JSON object")
	}
	if err := p.run(working, ops); err != nil {
		return err
	}
	out, err := json.Marshal(working)
	if err != nil {
		return scimerrors.ErrInternal("could not encode the patched resource")
	}
	return writeBack(resource, out)
}

func writeBack(resource any, doc []byte) error {
	value := reflect.ValueOf(resource)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return scimerrors.ErrInternal("resource must be a non-nil pointer")
	}
	elem := value.Elem()
	fresh := reflect.New(elem.Type())
	if err := json.Unmarshal(doc, fresh.Interface()); err != nil {
		return scimerrors.ErrInternal("could not decode the patched resource")
	}
	if elem.CanSet() {
		elem.Set(fresh.Elem())
	}
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

// RFC 7644 3.5.2.1 - a value written to a multi-valued attribute is an array.
func shaped(value any, multiValued bool) any {
	if !multiValued {
		return value
	}
	if list, ok := value.([]any); ok {
		return list
	}
	return []any{value}
}
