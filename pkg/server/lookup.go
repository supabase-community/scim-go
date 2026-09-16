package server

import "strings"

func lookup(doc any, segments []string) []any {
	if arr, ok := doc.([]any); ok {
		values := make([]any, 0, len(arr))
		for _, elem := range arr {
			values = append(values, lookup(elem, segments)...)
		}
		return values
	}
	if len(segments) == 0 {
		return []any{doc}
	}
	m, ok := doc.(map[string]any)
	if !ok {
		return nil
	}
	value, ok := lookupCI(m, segments[0])
	if !ok {
		return nil
	}
	return lookup(value, segments[1:])
}

func lookupCI(m map[string]any, key string) (any, bool) {
	if value, ok := m[key]; ok {
		return value, true
	}
	for k, value := range m {
		if strings.EqualFold(k, key) {
			return value, true
		}
	}
	return nil, false
}

func isEmpty(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return v == ""
	default:
		return false
	}
}
