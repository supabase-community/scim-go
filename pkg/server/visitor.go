package server

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type specification[T Entity] func(T) bool

type evaluator[T Entity] struct {
	getters Getters[T]
}

func NewVisitor[T Entity](getters Getters[T]) protocol.Evaluator[specification[T]] {
	return &evaluator[T]{getters: getters}
}

func (e *evaluator[T]) Compare(attribute *core.Attribute, key string, op filter.Operator, value any) (specification[T], error) {
	get, ok := e.getters[key]
	if !ok {
		return nil, scimerrors.ErrInvalidFilter(key + " is not filterable")
	}
	return func(item T) bool {
		return anyMatch(get(item), func(raw any) bool {
			return compareValue(op, raw, value, attribute.CaseExact)
		})
	}, nil
}

func (e *evaluator[T]) Present(attribute *core.Attribute, key string) (specification[T], error) {
	get, ok := e.getters[key]
	if !ok {
		return nil, scimerrors.ErrInvalidFilter(key + " is not filterable")
	}
	return func(item T) bool {
		return anyMatch(get(item), hasValue)
	}, nil
}

func (e *evaluator[T]) And(left, right specification[T]) (specification[T], error) {
	return func(item T) bool { return left(item) && right(item) }, nil
}

func (e *evaluator[T]) Or(left, right specification[T]) (specification[T], error) {
	return func(item T) bool { return left(item) || right(item) }, nil
}

func (e *evaluator[T]) Not(operand specification[T]) (specification[T], error) {
	return func(item T) bool { return !operand(item) }, nil
}

func (e *evaluator[T]) ValuePath(attribute *core.Attribute, key string, valueFilter func() (specification[T], error)) (specification[T], error) {
	return nil, scimerrors.ErrInvalidFilter(key + " does not support value filters yet")
}

// RFC 7644 3.4.2.2 - a multi-valued attribute matches if any value does, for every operator including ne.
func anyMatch(raw any, pred func(any) bool) bool {
	list, ok := raw.([]any)
	if !ok {
		return pred(raw)
	}
	return slices.ContainsFunc(list, pred)
}

// RFC 7644 3.4.2.2 - pr matches only a non-empty, non-null value.
func hasValue(raw any) bool {
	switch v := raw.(type) {
	case nil:
		return false
	case string:
		return v != ""
	default:
		return true
	}
}

func compareValue(op filter.Operator, got, want any, caseExact bool) bool {
	switch g := got.(type) {
	case string:
		w, ok := want.(string)
		return ok && compareStrings(op, g, w, caseExact)
	case bool:
		w, ok := want.(bool)
		return ok && compareBools(op, g, w)
	case int64:
		w, ok := want.(int64)
		return ok && compareOrdered(op, g, w)
	case float64:
		w, ok := want.(float64)
		return ok && compareOrdered(op, g, w)
	case time.Time:
		w, ok := want.(time.Time)
		return ok && compareTime(op, g, w)
	}
	return false
}

func compareStrings(op filter.Operator, got, want string, caseExact bool) bool {
	if !caseExact {
		got, want = strings.ToLower(got), strings.ToLower(want)
	}
	switch op {
	case filter.OpContains:
		return strings.Contains(got, want)
	case filter.OpStartsWith:
		return strings.HasPrefix(got, want)
	case filter.OpEndsWith:
		return strings.HasSuffix(got, want)
	default:
		return compareOrdered(op, got, want)
	}
}

func compareBools(op filter.Operator, got, want bool) bool {
	switch op {
	case filter.OpEquals:
		return got == want
	case filter.OpNotEquals:
		return got != want
	}
	return false
}

func compareOrdered[T cmp.Ordered](op filter.Operator, got, want T) bool {
	switch op {
	case filter.OpEquals:
		return got == want
	case filter.OpNotEquals:
		return got != want
	case filter.OpGreaterThan:
		return got > want
	case filter.OpGreaterThanEquals:
		return got >= want
	case filter.OpLessThan:
		return got < want
	case filter.OpLessThanEquals:
		return got <= want
	}
	return false
}

func compareTime(op filter.Operator, got, want time.Time) bool {
	switch op {
	case filter.OpEquals:
		return got.Equal(want)
	case filter.OpNotEquals:
		return !got.Equal(want)
	case filter.OpGreaterThan:
		return got.After(want)
	case filter.OpGreaterThanEquals:
		return !got.Before(want)
	case filter.OpLessThan:
		return got.Before(want)
	case filter.OpLessThanEquals:
		return !got.After(want)
	}
	return false
}
