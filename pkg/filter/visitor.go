package filter

import (
	"errors"
	"fmt"
)

type Visitor[Output any] interface {
	VisitAnd(left, right Output) (Output, error)
	VisitOr(left, right Output) (Output, error)
	VisitNot(operand Output) (Output, error)
	VisitEquals(attribute AttrPath, value any) (Output, error)
	VisitNotEquals(attribute AttrPath, value any) (Output, error)
	VisitContains(attribute AttrPath, value any) (Output, error)
	VisitStartsWith(attribute AttrPath, value any) (Output, error)
	VisitEndsWith(attribute AttrPath, value any) (Output, error)
	VisitGreaterThan(attribute AttrPath, value any) (Output, error)
	VisitGreaterThanEquals(attribute AttrPath, value any) (Output, error)
	VisitLessThan(attribute AttrPath, value any) (Output, error)
	VisitLessThanEquals(attribute AttrPath, value any) (Output, error)
	VisitPresence(attribute AttrPath) (Output, error)
	VisitValuePath(path AttrPath, valueFilter func() (Output, error)) (Output, error)
}

func Visit[Output any](v Visitor[Output], n *Node) (Output, error) {
	var zero Output
	if n == nil {
		return zero, errors.New("scim: cannot visit a nil node")
	}
	if n.Not() {
		operand, err := Visit(v, n.Operand())
		if err != nil {
			return zero, err
		}
		return v.VisitNot(operand)
	}
	return dispatch(v, n)
}

func dispatch[Output any](v Visitor[Output], n *Node) (Output, error) {
	if n.HasPath() {
		return v.VisitValuePath(n.AttrPath(), func() (Output, error) {
			return Visit(v, n.ValueFilter())
		})
	}

	switch Operator(n.Operator()) {
	case OpAnd:
		return visitBinary(v, n, v.VisitAnd)
	case OpOr:
		return visitBinary(v, n, v.VisitOr)
	case OpPresent:
		return v.VisitPresence(n.AttrPath())
	}
	return dispatchComparison(v, n)
}

func dispatchComparison[Output any](v Visitor[Output], n *Node) (Output, error) {
	var zero Output

	attr, val := n.AttrPath(), n.Value()
	switch Operator(n.Operator()) {
	case OpEquals:
		return v.VisitEquals(attr, val)
	case OpNotEquals:
		return v.VisitNotEquals(attr, val)
	case OpContains:
		return v.VisitContains(attr, val)
	case OpStartsWith:
		return v.VisitStartsWith(attr, val)
	case OpEndsWith:
		return v.VisitEndsWith(attr, val)
	case OpGreaterThan:
		return v.VisitGreaterThan(attr, val)
	case OpGreaterThanEquals:
		return v.VisitGreaterThanEquals(attr, val)
	case OpLessThan:
		return v.VisitLessThan(attr, val)
	case OpLessThanEquals:
		return v.VisitLessThanEquals(attr, val)
	default:
		return zero, fmt.Errorf("scim: unrecognized node shape: %+v", n.raw)
	}
}

func visitBinary[Output any](v Visitor[Output], n *Node, combine func(left, right Output) (Output, error)) (Output, error) {
	var zero Output
	left, err := Visit(v, n.Left())
	if err != nil {
		return zero, err
	}
	right, err := Visit(v, n.Right())
	if err != nil {
		return zero, err
	}
	return combine(left, right)
}
