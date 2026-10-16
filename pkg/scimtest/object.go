package scimtest

import (
	"slices"

	"github.com/supabase-community/scim-go/pkg/core"
)

func idOf(object core.Object) string {
	id, _ := object.Get("id").(string)
	return id
}

func idsOf(objects []core.Object) []string {
	ids := make([]string, 0, len(objects))
	for _, object := range objects {
		ids = append(ids, idOf(object))
	}
	return ids
}

func metaOf(object core.Object) map[string]any {
	meta, _ := object.Get("meta").(map[string]any)
	return meta
}

func objectOf(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}

func elementsOf(value any) []map[string]any {
	list, _ := value.([]any)
	elements := make([]map[string]any, 0, len(list))
	for _, element := range list {
		if object, ok := element.(map[string]any); ok {
			elements = append(elements, object)
		}
	}
	return elements
}

func valuesIn(object core.Object, name string) []string {
	values := []string{}
	for _, element := range elementsOf(object.Get(name)) {
		if value, ok := element[valueSub].(string); ok {
			values = append(values, value)
		}
	}
	slices.Sort(values)
	return values
}

func members(ids ...string) []any {
	values := make([]any, 0, len(ids))
	for _, id := range ids {
		values = append(values, map[string]any{valueSub: id})
	}
	return values
}

func isEmptyObject(value any) bool {
	object, ok := value.(map[string]any)
	return value == nil || ok && len(object) == 0
}
