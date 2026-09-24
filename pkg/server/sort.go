package server

import (
	"cmp"
	"strings"
	"time"
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
func compareSortKeys(a, b any, caseExact, descending bool) int {
	result := compareAscending(a, b, caseExact)
	if descending {
		return -result
	}
	return result
}

func compareAscending(a, b any, caseExact bool) int {
	aMissing, bMissing := isMissingSortValue(a), isMissingSortValue(b)
	switch {
	case aMissing && bMissing:
		return 0
	case aMissing:
		return 1
	case bMissing:
		return -1
	}
	return compareSortValue(a, b, caseExact)
}

func isMissingSortValue(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return v == ""
	default:
		return false
	}
}

func compareSortValue(a, b any, caseExact bool) int {
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		if !ok {
			return 0
		}
		if !caseExact {
			av, bv = strings.ToLower(av), strings.ToLower(bv)
		}
		return cmp.Compare(av, bv)
	case bool:
		bv, ok := b.(bool)
		if !ok {
			return 0
		}
		return cmp.Compare(boolSortRank(av), boolSortRank(bv))
	case int64:
		bv, ok := b.(int64)
		if !ok {
			return 0
		}
		return cmp.Compare(av, bv)
	case float64:
		bv, ok := b.(float64)
		if !ok {
			return 0
		}
		return cmp.Compare(av, bv)
	case time.Time:
		bv, ok := b.(time.Time)
		if !ok {
			return 0
		}
		return av.Compare(bv)
	default:
		return 0
	}
}

func boolSortRank(v bool) int {
	if v {
		return 1
	}
	return 0
}
