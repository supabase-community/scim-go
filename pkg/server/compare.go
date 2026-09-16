package server

import (
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
)

func compareOne(attr *core.Attribute, op filter.Operator, candidate, value any) bool {
	switch op {
	case filter.OpEquals:
		return equalValues(attr, candidate, value)
	case filter.OpNotEquals:
		return !equalValues(attr, candidate, value)
	case filter.OpContains, filter.OpStartsWith, filter.OpEndsWith:
		cs, vs, ok := asComparableStrings(attr, candidate, value)
		if !ok {
			return false
		}
		switch op {
		case filter.OpContains:
			return strings.Contains(cs, vs)
		case filter.OpStartsWith:
			return strings.HasPrefix(cs, vs)
		default:
			return strings.HasSuffix(cs, vs)
		}
	default: // gt, ge, lt, le
		cs, vs, ok := asComparableStrings(attr, candidate, value)
		if !ok {
			return false
		}
		switch op {
		case filter.OpGreaterThan:
			return cs > vs
		case filter.OpGreaterThanEquals:
			return cs >= vs
		case filter.OpLessThan:
			return cs < vs
		default:
			return cs <= vs
		}
	}
}

func asComparableStrings(attr *core.Attribute, candidate, value any) (cs, vs string, ok bool) {
	cs, ok1 := candidate.(string)
	vs, ok2 := value.(string)
	if !ok1 || !ok2 {
		return "", "", false
	}
	if !attr.CaseExact {
		cs, vs = strings.ToLower(cs), strings.ToLower(vs)
	}
	return cs, vs, true
}

func equalValues(attr *core.Attribute, candidate, value any) bool {
	switch v := value.(type) {
	case string:
		cs, ok := candidate.(string)
		if !ok {
			return false
		}
		if attr.CaseExact {
			return cs == v
		}
		return strings.EqualFold(cs, v)
	case bool:
		cb, ok := candidate.(bool)
		return ok && cb == v
	default:
		return candidate == value
	}
}
