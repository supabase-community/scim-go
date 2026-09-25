package patch

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type matcher struct {
	attr   *core.Attribute
	budget *budget
}

func compile(attr *core.Attribute, node *filter.Node, budget *budget) (predicate, error) {
	pred, err := filter.Visit[predicate](matcher{attr: attr, budget: budget}, node)
	var scimErr *scimerrors.Error
	if err != nil && !errors.As(err, &scimErr) {
		return nil, scimerrors.ErrInvalidPath(err.Error())
	}
	return pred, err
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
	return m.leaf(attr, filter.OpEquals, value)
}

func (m matcher) VisitNotEquals(attr filter.AttrPath, value any) (predicate, error) {
	return m.leaf(attr, filter.OpNotEquals, value)
}

func (m matcher) VisitContains(attr filter.AttrPath, value any) (predicate, error) {
	return m.leaf(attr, filter.OpContains, value)
}

func (m matcher) VisitStartsWith(attr filter.AttrPath, value any) (predicate, error) {
	return m.leaf(attr, filter.OpStartsWith, value)
}

func (m matcher) VisitEndsWith(attr filter.AttrPath, value any) (predicate, error) {
	return m.leaf(attr, filter.OpEndsWith, value)
}

func (m matcher) VisitGreaterThan(attr filter.AttrPath, value any) (predicate, error) {
	return m.leaf(attr, filter.OpGreaterThan, value)
}

func (m matcher) VisitGreaterThanEquals(attr filter.AttrPath, value any) (predicate, error) {
	return m.leaf(attr, filter.OpGreaterThanEquals, value)
}

func (m matcher) VisitLessThan(attr filter.AttrPath, value any) (predicate, error) {
	return m.leaf(attr, filter.OpLessThan, value)
}

func (m matcher) VisitLessThanEquals(attr filter.AttrPath, value any) (predicate, error) {
	return m.leaf(attr, filter.OpLessThanEquals, value)
}

func (m matcher) VisitPresence(path filter.AttrPath) (predicate, error) {
	sub, err := m.resolve(path)
	if err != nil {
		return nil, err
	}
	if value.Hidden(m.attr, sub) {
		return nil, scimerrors.ErrInvalidFilter(fmt.Sprintf("operator \"pr\" is not valid for %q", path.String()))
	}
	return func(member map[string]any) bool {
		return m.budget.spend() && !value.IsUnassigned(core.Object(member).Get(path.Name))
	}, nil
}

// RFC 7644 3.4.2.2 - a value filter cannot itself contain a value path.
func (m matcher) VisitValuePath(_ filter.AttrPath, _ string, _ func() (predicate, error)) (predicate, error) {
	return nil, scimerrors.ErrInvalidPath("value filter cannot contain a nested value path")
}

func (m matcher) leaf(path filter.AttrPath, op filter.Operator, want any) (predicate, error) {
	attr, err := m.resolve(path)
	if err != nil {
		return nil, err
	}
	if err := m.check(attr, path, op, want); err != nil {
		return nil, err
	}
	return func(member map[string]any) bool {
		if !m.budget.spend() {
			return false
		}
		got := core.Object(member).Get(path.Name)
		typed := cmp.Or(attr, inferred(got))
		expected, ok := literal(typed, op, want)
		actual, _ := typed.Coerce(got)
		return ok && value.Match(op, value.Fold(typed, actual), expected)
	}, nil
}

// RFC 7644 Section 3.12, Table 9: a value filter names a sub-attribute of the multi-valued attribute; without schemas any name is accepted.
func (m matcher) resolve(path filter.AttrPath) (*core.Attribute, error) {
	sub := m.attr.SubAttribute(path.Name)
	if m.attr != permissiveAttr && (sub == nil || path.SubAttribute != "") {
		return nil, scimerrors.ErrInvalidFilter(fmt.Sprintf("%q is not a known attribute", path.String()))
	}
	return sub, nil
}

func (m matcher) check(attr *core.Attribute, path filter.AttrPath, op filter.Operator, want any) error {
	switch {
	case attr == nil:
		return nil
	case !value.Allowed(attr.Type, op) || (value.Hidden(m.attr, attr) && op != filter.OpEquals):
		return scimerrors.ErrInvalidFilter(fmt.Sprintf("operator %q is not valid for %q", op, path.String()))
	}
	if _, ok := attr.Coerce(want); !ok {
		return scimerrors.ErrInvalidValue(fmt.Sprintf("%q is not a valid value for %q", want, path.String()))
	}
	return nil
}

func literal(attr *core.Attribute, op filter.Operator, want any) (any, bool) {
	coerced, ok := attr.Coerce(want)
	return value.Fold(attr, coerced), ok && value.Allowed(attr.Type, op)
}

func inferred(got any) *core.Attribute {
	switch got.(type) {
	case string:
		return core.NewAttribute("", core.TypeString)
	case bool:
		return core.NewAttribute("", core.TypeBoolean)
	case json.Number, float64, int64, int:
		return core.NewAttribute("", core.TypeDecimal)
	}
	return permissiveAttr
}
