package server

import (
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type specification[T Entity] func(T) bool
type getter[T any] func(T) any
type getters[T any] map[string]getter[T]

type evaluator[T Entity] struct {
	getters getters[T]
}

func NewVisitor[T Entity](getters getters[T]) protocol.Evaluator[specification[T]] {
	return &evaluator[T]{
		getters: getters,
	}
}

func (e *evaluator[T]) Compare(attribute *core.Attribute, key string, op filter.Operator, value any) (specification[T], error) {
	return func(item T) bool {
		got := e.getters[attribute.Name](item)
		switch op {
		case filter.OpEquals:
			return got == value
		}
		return false
	}, nil
}

func (e *evaluator[T]) Present(attribute *core.Attribute, key string) (specification[T], error) {
	return func(item T) bool {
		return false
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

func (e *evaluator[T]) value(item T, attribute *core.Attribute, key string) any {
	return nil
}
