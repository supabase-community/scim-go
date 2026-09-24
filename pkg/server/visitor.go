package server

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type predicate func(row any) bool

type evaluator struct {
	readers
}

func newVisitor(readers readers) protocol.Evaluator[predicate] {
	return &evaluator{readers: readers}
}

func (e *evaluator) Compare(attribute *protocol.Attribute, op filter.Operator, value any) (predicate, error) {
	read, err := e.reader(attribute)
	if err != nil {
		return nil, err
	}
	return func(row any) bool {
		return anyMatch(read(row), func(raw any) bool {
			return compareValue(op, raw, value, attribute.Definition.CaseExact)
		})
	}, nil
}

func (e *evaluator) Present(attribute *protocol.Attribute) (predicate, error) {
	read, err := e.reader(attribute)
	if err != nil {
		return nil, err
	}
	return func(row any) bool {
		return anyMatch(read(row), hasValue)
	}, nil
}

func (e *evaluator) And(left, right predicate) (predicate, error) {
	return func(row any) bool { return left(row) && right(row) }, nil
}

func (e *evaluator) Or(left, right predicate) (predicate, error) {
	return func(row any) bool { return left(row) || right(row) }, nil
}

func (e *evaluator) Not(operand predicate) (predicate, error) {
	return func(row any) bool { return !operand(row) }, nil
}

func (e *evaluator) ValuePath(attribute *protocol.Attribute, valueFilter func() (predicate, error)) (predicate, error) {
	elements, ok := e.elements[attribute.Definition]
	if !ok {
		return nil, scimerrors.ErrInvalidFilter(attribute.Path.Key() + " is not filterable")
	}
	inner, err := valueFilter()
	if err != nil {
		return nil, err
	}
	return func(row any) bool {
		return slices.ContainsFunc(elements(asDocument(row)), inner)
	}, nil
}

func (e *evaluator) reader(attribute *protocol.Attribute) (func(row any) any, error) {
	definition := attribute.Definition
	if attribute.Parent != nil {
		return func(row any) any { return coerce(definition, asDocument(row).get(definition.Name)) }, nil
	}
	if read, ok := e.values[definition]; ok {
		return func(row any) any { return read(asDocument(row)) }, nil
	}
	return nil, scimerrors.ErrInvalidFilter(attribute.Path.Key() + " is not filterable")
}

// RFC 7644 3.4.2.2 - a multi-valued attribute matches if any value does, for every operator including ne.
func anyMatch(raw any, pred func(any) bool) bool {
	list, ok := raw.([]any)
	if !ok {
		return pred(raw)
	}
	return slices.ContainsFunc(list, pred)
}

// RFC 7644 3.4.2.2 - pr matches only a non-empty, non-null value or a non-empty complex node.
func hasValue(raw any) bool {
	switch v := raw.(type) {
	case nil:
		return false
	case string:
		return v != ""
	case map[string]any:
		return len(v) > 0
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
