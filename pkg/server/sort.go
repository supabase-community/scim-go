package server

import (
	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
)

func primaryOrFirst(elements []any) (any, bool) {
	for _, element := range elements {
		if asObject(element).Get("primary") == true {
			return element, true
		}
	}
	if len(elements) == 0 {
		return nil, false
	}
	return elements[0], true
}

// RFC 7644 Section 3.4.2.3: resources without a value are ordered last if ascending and first if descending.
func compareSortKeys(attribute *core.Attribute, a, b any, descending bool) int {
	result := compareAscending(attribute, a, b)
	if descending {
		return -result
	}
	return result
}

func compareAscending(attribute *core.Attribute, a, b any) int {
	aMissing, bMissing := value.IsUnassigned(a), value.IsUnassigned(b)
	switch {
	case aMissing && bMissing:
		return 0
	case aMissing:
		return 1
	case bMissing:
		return -1
	}
	order, _ := value.Compare(value.Fold(attribute, a), value.Fold(attribute, b))
	return order
}
