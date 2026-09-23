package filter

import (
	"fmt"
	"strings"
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
	VisitValuePath(path AttrPath, subAttribute string, valueFilter func() (Output, error)) (Output, error)
}

func Visit[Output any](v Visitor[Output], n *Node) (Output, error) {
	var zero Output
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

func dispatch[Output any](v Visitor[Output], n *Node) (Output, error) {
	var zero Output

	if n.HasPath() {
		return v.VisitValuePath(n.AttrPath(), n.SubAttribute(), func() (Output, error) {
			return Visit(v, n.ValueFilter())
		})
	}

	attr := n.AttrPath()
	switch Operator(strings.ToLower(n.Operator())) {
	case "and":
		return visitBinary(v, n, v.VisitAnd)
	case "or":
		return visitBinary(v, n, v.VisitOr)
	case OpEquals:
		return v.VisitEquals(attr, n.Value())
	case OpNotEquals:
		return v.VisitNotEquals(attr, n.Value())
	case OpContains:
		return v.VisitContains(attr, n.Value())
	case OpStartsWith:
		return v.VisitStartsWith(attr, n.Value())
	case OpEndsWith:
		return v.VisitEndsWith(attr, n.Value())
	case OpGreaterThan:
		return v.VisitGreaterThan(attr, n.Value())
	case OpGreaterThanEquals:
		return v.VisitGreaterThanEquals(attr, n.Value())
	case OpLessThan:
		return v.VisitLessThan(attr, n.Value())
	case OpLessThanEquals:
		return v.VisitLessThanEquals(attr, n.Value())
	case "pr":
		return v.VisitPresence(attr)
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
