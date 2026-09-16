package server

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/supabase-community/scim-go/pkg/core"
)

func cloneResource[T core.Resource](item T) (T, error) {
	var zero T
	raw, err := json.Marshal(item)
	if err != nil {
		return zero, err
	}
	clone, ok := reflect.New(reflect.TypeOf(item).Elem()).Interface().(T)
	if !ok {
		return zero, fmt.Errorf("scim: %T is not a pointer to a resource", item)
	}
	if err := json.Unmarshal(raw, clone); err != nil {
		return zero, err
	}
	return clone, nil
}
