package server

import (
	"slices"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type predicate func(row any) bool

type evaluator struct {
	fields
}

func newEvaluator(fields fields) protocol.Evaluator[predicate] {
	return &evaluator{fields: fields}
}

func (e *evaluator) Compare(attribute *protocol.Attribute, op filter.Operator, literal any) (predicate, error) {
	read, err := e.reader(attribute)
	if err != nil {
		return nil, err
	}
	definition := attribute.Definition
	want := value.Fold(definition, literal)
	return func(row any) bool {
		return anyMatch(read(row), func(raw any) bool { return value.Match(op, value.Fold(definition, raw), want) })
	}, nil
}

func (e *evaluator) Present(attribute *protocol.Attribute) (predicate, error) {
	read, err := e.reader(attribute)
	if err != nil {
		return nil, err
	}
	return func(row any) bool {
		return anyMatch(read(row), func(v any) bool { return !value.IsUnassigned(v) })
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
	list, ok := e.lookup(attribute.Definition)
	if !ok || !list.isList() {
		return nil, scimerrors.ErrInvalidFilter(attribute.Path.Key() + " is not filterable")
	}
	inner, err := valueFilter()
	if err != nil {
		return nil, err
	}
	return func(row any) bool {
		return slices.ContainsFunc(list.elements(asObject(row)), inner)
	}, nil
}

func (e *evaluator) reader(attribute *protocol.Attribute) (func(row any) any, error) {
	definition := attribute.Definition
	if attribute.Parent != nil {
		return func(row any) any { return coerce(definition, asObject(row).Get(definition.Name)) }, nil
	}
	if field, ok := e.lookup(definition); ok {
		return func(row any) any { return field.value(asObject(row)) }, nil
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
