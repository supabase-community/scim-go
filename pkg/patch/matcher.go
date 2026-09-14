package patch

import (
	"encoding/json"
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type matcher struct{}

func (m matcher) compile(node *filter.Node, attr *core.Attribute) (predicate, error) {
	switch {
	case node == nil:
		return nil, scimerrors.ErrInvalidPath("empty value filter")
	case node.Not():
		return m.compileNot(node, attr)
	case node.Left() != nil:
		return m.compileBinary(node, attr)
	default:
		return m.compileLeaf(node, attr), nil
	}
}

func (m matcher) compileNot(node *filter.Node, attr *core.Attribute) (predicate, error) {
	inner, err := m.compile(node.Operand(), attr)
	if err != nil {
		return nil, err
	}
	return func(member map[string]any) bool { return !inner(member) }, nil
}

func (m matcher) compileBinary(node *filter.Node, attr *core.Attribute) (predicate, error) {
	left, err := m.compile(node.Left(), attr)
	if err != nil {
		return nil, err
	}
	right, err := m.compile(node.Right(), attr)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(node.Operator(), "or") {
		return func(member map[string]any) bool { return left(member) || right(member) }, nil
	}
	return func(member map[string]any) bool { return left(member) && right(member) }, nil
}

func (m matcher) compileLeaf(node *filter.Node, attr *core.Attribute) predicate {
	l := leaf{
		key:       node.AttrPath().Name,
		op:        strings.ToLower(node.Operator()),
		want:      node.Value(),
		caseExact: subAttr(attr, node.AttrPath().Name).CaseExact,
	}
	return func(member map[string]any) bool { return m.matchOne(member, l) }
}

func (m matcher) matchOne(member map[string]any, l leaf) bool {
	got, ok := object(member).get(l.key)
	if l.op == "pr" {
		return ok && got != nil
	}
	if !ok || got == nil {
		return false
	}
	return m.compareValues(filter.Operator(l.op), got, l.want, l.caseExact)
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
	switch n := value.(type) {
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case float64:
		return n, true
	case int64:
		return float64(n), true
	}
	return 0, false
}
