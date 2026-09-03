package filter

import (
	"fmt"
	"strings"
)

type Visitor[T any] interface {
	VisitAnd(left, right T) (T, error)
	VisitOr(left, right T) (T, error)
	VisitNot(operand T) (T, error)
	VisitEquals(attribute AttrPath, value any) (T, error)
	VisitNotEquals(attribute AttrPath, value any) (T, error)
	VisitContains(attribute AttrPath, value any) (T, error)
	VisitStartsWith(attribute AttrPath, value any) (T, error)
	VisitEndsWith(attribute AttrPath, value any) (T, error)
	VisitGreaterThan(attribute AttrPath, value any) (T, error)
	VisitGreaterThanEquals(attribute AttrPath, value any) (T, error)
	VisitLessThan(attribute AttrPath, value any) (T, error)
	VisitLessThanEquals(attribute AttrPath, value any) (T, error)
	VisitPresence(attribute AttrPath) (T, error)
	VisitValuePath(path AttrPath, subAttribute string, valueFilter func() (T, error)) (T, error)
}

func Visit[T any](v Visitor[T], n *Node) (T, error) {
	var zero T
	if n == nil {
		return zero, fmt.Errorf("scim: cannot visit a nil node")
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

func dispatch[T any](v Visitor[T], n *Node) (T, error) {
	var zero T

	if n.HasPath() {
		return v.VisitValuePath(n.AttrPath(), n.SubAttribute(), func() (T, error) {
			return Visit(v, n.ValueFilter())
		})
	}

	attr := n.AttrPath()
	switch strings.ToLower(n.Operator()) {
	case "and":
		return visitBinary(v, n, v.VisitAnd)
	case "or":
		return visitBinary(v, n, v.VisitOr)
	case "eq":
		return v.VisitEquals(attr, n.Value())
	case "ne":
		return v.VisitNotEquals(attr, n.Value())
	case "co":
		return v.VisitContains(attr, n.Value())
	case "sw":
		return v.VisitStartsWith(attr, n.Value())
	case "ew":
		return v.VisitEndsWith(attr, n.Value())
	case "gt":
		return v.VisitGreaterThan(attr, n.Value())
	case "ge":
		return v.VisitGreaterThanEquals(attr, n.Value())
	case "lt":
		return v.VisitLessThan(attr, n.Value())
	case "le":
		return v.VisitLessThanEquals(attr, n.Value())
	case "pr":
		return v.VisitPresence(attr)
	default:
		return zero, fmt.Errorf("scim: unrecognized node shape: %+v", n.raw)
	}
}

func visitBinary[T any](v Visitor[T], n *Node, combine func(left, right T) (T, error)) (T, error) {
	var zero T
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
