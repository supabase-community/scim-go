package value

import (
	"cmp"
	"reflect"
	"strings"
	"time"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
)

// Allowed reports whether op applies to an attribute of type t, per RFC 7644, Section 3.4.2.2.
func Allowed(t core.AttributeType, op filter.Operator) bool {
	switch op {
	case filter.OpEquals, filter.OpNotEquals:
		return true
	case filter.OpContains, filter.OpStartsWith, filter.OpEndsWith:
		return t == core.TypeString || t == core.TypeReference
	case filter.OpGreaterThan, filter.OpGreaterThanEquals, filter.OpLessThan, filter.OpLessThanEquals:
		return t == core.TypeString || t == core.TypeDateTime || t == core.TypeInteger || t == core.TypeDecimal
	}
	return false
}

// Fold lowercases a string value unless it is case exact; binary and reference values always are, per RFC 7643, Sections 2.3.6 and 2.3.7.
func Fold(attribute *core.Attribute, value any) any {
	s, ok := value.(string)
	if !ok || attribute.CaseExact || attribute.Type == core.TypeBinary || attribute.Type == core.TypeReference {
		return value
	}
	return strings.ToLower(s)
}

// Equal reports whether a and b are the same value of attribute, per RFC 7643, Section 2.3.
func Equal(attribute *core.Attribute, a, b any) bool {
	if order, ok := Compare(Fold(attribute, a), Fold(attribute, b)); ok {
		return order == 0
	}
	return reflect.DeepEqual(a, b)
}

// Match reports whether got op want holds for two folded values, per RFC 7644, Section 3.4.2.2.
func Match(op filter.Operator, got, want any) bool {
	switch op {
	case filter.OpContains, filter.OpStartsWith, filter.OpEndsWith:
		return substring(op, got, want)
	}
	order, ok := Compare(got, want)
	return ok && holds(op, order)
}

// Compare orders two folded values of the same type; ok is false when their types differ.
func Compare(a, b any) (int, bool) {
	switch x := a.(type) {
	case string:
		return compare(x, b)
	case int64:
		return compare(x, b)
	case float64:
		return compare(x, b)
	case bool:
		return compare(rank(x), rankOf(b))
	case time.Time:
		y, ok := b.(time.Time)
		if !ok {
			return 0, false
		}
		return x.Compare(y), true
	}
	return 0, false
}

func compare[T cmp.Ordered](x T, b any) (int, bool) {
	y, ok := b.(T)
	if !ok {
		return 0, false
	}
	return cmp.Compare(x, y), true
}

func rank(b bool) int {
	if b {
		return 1
	}
	return 0
}

func rankOf(b any) any {
	if y, ok := b.(bool); ok {
		return rank(y)
	}
	return nil
}

func substring(op filter.Operator, got, want any) bool {
	g, gok := got.(string)
	w, wok := want.(string)
	switch {
	case !gok || !wok:
		return false
	case op == filter.OpContains:
		return strings.Contains(g, w)
	case op == filter.OpStartsWith:
		return strings.HasPrefix(g, w)
	}
	return strings.HasSuffix(g, w)
}

func holds(op filter.Operator, order int) bool {
	switch op {
	case filter.OpEquals:
		return order == 0
	case filter.OpNotEquals:
		return order != 0
	case filter.OpGreaterThan:
		return order > 0
	case filter.OpGreaterThanEquals:
		return order >= 0
	case filter.OpLessThan:
		return order < 0
	case filter.OpLessThanEquals:
		return order <= 0
	}
	return false
}
