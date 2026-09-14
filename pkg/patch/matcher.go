package patch

import (
	"encoding/json"
	"reflect"
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

const opPresent = filter.Operator("pr")

type matcher struct {
	attr *core.Attribute
}

func compile(attr *core.Attribute, node *filter.Node) (predicate, error) {
	pred, err := filter.Visit[predicate](matcher{attr: attr}, node)
	if err != nil {
		return nil, scimerrors.ErrInvalidPath(err.Error())
	}
	return pred, nil
}

func (m matcher) VisitAnd(left, right predicate) (predicate, error) {
	return func(member map[string]any) bool { return left(member) && right(member) }, nil
}

func (m matcher) VisitOr(left, right predicate) (predicate, error) {
	return func(member map[string]any) bool { return left(member) || right(member) }, nil
}

func (m matcher) VisitNot(operand predicate) (predicate, error) {
	return func(member map[string]any) bool { return !operand(member) }, nil
}

func (m matcher) VisitEquals(attr filter.AttrPath, value any) (predicate, error) {
	return m.leaf(attr, filter.OpEquals, value), nil
}

func (m matcher) VisitNotEquals(attr filter.AttrPath, value any) (predicate, error) {
	return m.leaf(attr, filter.OpNotEquals, value), nil
}

func (m matcher) VisitContains(attr filter.AttrPath, value any) (predicate, error) {
	return m.leaf(attr, filter.OpContains, value), nil
}

func (m matcher) VisitStartsWith(attr filter.AttrPath, value any) (predicate, error) {
	return m.leaf(attr, filter.OpStartsWith, value), nil
}

func (m matcher) VisitEndsWith(attr filter.AttrPath, value any) (predicate, error) {
	return m.leaf(attr, filter.OpEndsWith, value), nil
}

func (m matcher) VisitGreaterThan(attr filter.AttrPath, value any) (predicate, error) {
	return m.leaf(attr, filter.OpGreaterThan, value), nil
}

func (m matcher) VisitGreaterThanEquals(attr filter.AttrPath, value any) (predicate, error) {
	return m.leaf(attr, filter.OpGreaterThanEquals, value), nil
}

func (m matcher) VisitLessThan(attr filter.AttrPath, value any) (predicate, error) {
	return m.leaf(attr, filter.OpLessThan, value), nil
}

func (m matcher) VisitLessThanEquals(attr filter.AttrPath, value any) (predicate, error) {
	return m.leaf(attr, filter.OpLessThanEquals, value), nil
}

func (m matcher) VisitPresence(attr filter.AttrPath) (predicate, error) {
	return m.leaf(attr, opPresent, nil), nil
}

// RFC 7644 3.4.2.2 - a value filter cannot itself contain a value path.
func (m matcher) VisitValuePath(_ filter.AttrPath, _ string, _ func() (predicate, error)) (predicate, error) {
	return nil, scimerrors.ErrInvalidPath("value filter cannot contain a nested value path")
}

func (m matcher) leaf(attr filter.AttrPath, op filter.Operator, want any) predicate {
	key := attr.Name
	caseExact := false
	if sub := m.attr.SubAttribute(key); sub != nil {
		caseExact = sub.CaseExact
	}
	return func(member map[string]any) bool {
		got, ok := object(member).get(key)
		if op == opPresent {
			return ok && hasValue(got)
		}
		if !ok || got == nil {
			return false
		}
		return m.compareValues(op, got, want, caseExact)
	}
}

func (m matcher) compareValues(op filter.Operator, got, want any, caseExact bool) bool {
	if gs, ok := got.(string); ok {
		ws, ok := want.(string)
		return ok && m.compareStrings(op, gs, ws, caseExact)
	}
	if gb, ok := got.(bool); ok {
		wb, ok := want.(bool)
		return ok && m.compareBools(op, gb, wb)
	}
	gf, gok := m.toFloat(got)
	wf, wok := m.toFloat(want)
	return gok && wok && m.compareNumbers(op, gf, wf)
}

func (m matcher) compareStrings(op filter.Operator, got, want string, caseExact bool) bool {
	if !caseExact {
		got, want = strings.ToLower(got), strings.ToLower(want)
	}
	switch op {
	case filter.OpEquals:
		return got == want
	case filter.OpNotEquals:
		return got != want
	case filter.OpContains:
		return strings.Contains(got, want)
	case filter.OpStartsWith:
		return strings.HasPrefix(got, want)
	case filter.OpEndsWith:
		return strings.HasSuffix(got, want)
	case filter.OpGreaterThan:
		return got > want
	case filter.OpLessThan:
		return got < want
	case filter.OpGreaterThanEquals:
		return got >= want
	case filter.OpLessThanEquals:
		return got <= want
	}
	return false
}

func (m matcher) compareBools(op filter.Operator, got, want bool) bool {
	switch op {
	case filter.OpEquals:
		return got == want
	case filter.OpNotEquals:
		return got != want
	}
	return false
}

func (m matcher) compareNumbers(op filter.Operator, got, want float64) bool {
	switch op {
	case filter.OpEquals:
		return got == want
	case filter.OpNotEquals:
		return got != want
	case filter.OpGreaterThan:
		return got > want
	case filter.OpLessThan:
		return got < want
	case filter.OpGreaterThanEquals:
		return got >= want
	case filter.OpLessThanEquals:
		return got <= want
	}
	return false
}

func (m matcher) toFloat(value any) (float64, bool) {
	if n, ok := value.(json.Number); ok {
		f, err := n.Float64()
		return f, err == nil
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Float32, reflect.Float64:
		return v.Float(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(v.Uint()), true
	}
	return 0, false
}

// RFC 7644 3.4.2.2 - pr matches only a non-empty, non-null value.
func hasValue(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case string:
		return v != ""
	case []any:
		return len(v) > 0
	case map[string]any:
		return len(v) > 0
	default:
		return true
	}
}
