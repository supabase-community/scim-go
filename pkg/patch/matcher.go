package patch

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

var (
	inferredString  = core.NewAttribute("", core.TypeString)
	inferredBoolean = core.NewAttribute("", core.TypeBoolean)
	inferredDecimal = core.NewAttribute("", core.TypeDecimal)
)

type predicate func(any) bool

type matcher struct {
	attr    *core.Attribute
	clauses *int
}

func compile(attr *core.Attribute, node *filter.Node) (predicate, int, error) {
	if attr != permissiveAttr && !attr.MultiValued {
		return nil, 0, scimerrors.ErrInvalidFilter(scimerrors.InvalidFilter.Description())
	}
	m := matcher{attr: attr, clauses: new(int)}
	pred, err := filter.Visit[predicate](m, node)
	var scimErr *scimerrors.Error
	if err != nil && !errors.As(err, &scimErr) {
		return nil, 0, scimerrors.ErrInvalidPath(scimerrors.InvalidPath.Description())
	}
	return pred, *m.clauses, err
}

func (m matcher) VisitAnd(left, right predicate) (predicate, error) {
	return func(element any) bool { return left(element) && right(element) }, nil
}

func (m matcher) VisitOr(left, right predicate) (predicate, error) {
	return func(element any) bool { return left(element) || right(element) }, nil
}

func (m matcher) VisitNot(operand predicate) (predicate, error) {
	*m.clauses++
	return func(element any) bool { return !operand(element) }, nil
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
		return nil, scimerrors.ErrInvalidFilter(scimerrors.InvalidFilter.Description())
	}
	*m.clauses++
	return func(element any) bool { return !value.IsUnassigned(field(element, path.Name)) }, nil
}

// RFC 7644 3.4.2.2 - a value filter cannot itself contain a value path.
func (m matcher) VisitValuePath(_ filter.AttrPath, _ func() (predicate, error)) (predicate, error) {
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
	*m.clauses++
	if attr != nil {
		expected, ok := literal(attr, op, want)
		return func(element any) bool {
			actual, _ := attr.Coerce(field(element, path.Name))
			return ok && value.Match(op, value.Fold(attr, actual), expected)
		}, nil
	}
	return func(element any) bool {
		got := field(element, path.Name)
		typed := inferred(got)
		if got == nil {
			typed = inferred(want)
		}
		expected, ok := literal(typed, op, want)
		actual, _ := typed.Coerce(got)
		return ok && value.Match(op, value.Fold(typed, actual), expected)
	}, nil
}

// RFC 7644 Section 3.4.2.2: a value filter MUST be a valid filter expression based upon sub-attributes of the parent attribute.
func (m matcher) resolve(path filter.AttrPath) (*core.Attribute, error) {
	if isSimple(m.attr) && path.URI == "" && path.SubAttribute == "" && strings.EqualFold(path.Name, "value") {
		return m.attr, nil
	}
	sub := m.attr.SubAttribute(path.Name)
	if m.attr != permissiveAttr && (sub == nil || path.URI != "" || path.SubAttribute != "") {
		return nil, scimerrors.ErrInvalidFilter(scimerrors.InvalidFilter.Description())
	}
	return sub, nil
}

func (m matcher) check(attr *core.Attribute, path filter.AttrPath, op filter.Operator, want any) error {
	switch {
	case attr == nil:
		return nil
	case !value.Allowed(attr.Type, op) || (value.Hidden(m.attr, attr) && op != filter.OpEquals):
		return scimerrors.ErrInvalidFilter(scimerrors.InvalidFilter.Description())
	}
	if _, ok := attr.Coerce(want); !ok {
		return scimerrors.ErrInvalidValue(scimerrors.InvalidValue.Description())
	}
	return nil
}

func literal(attr *core.Attribute, op filter.Operator, want any) (any, bool) {
	coerced, ok := attr.Coerce(want)
	return value.Fold(attr, coerced), ok && value.Allowed(attr.Type, op)
}

func isSimple(attr *core.Attribute) bool {
	return attr != permissiveAttr && attr.Type != core.TypeComplex
}

// RFC 7644 Section 3.5.2.2: a filter comparing "value" matches the values of a simple multi-valued attribute.
func field(element any, name string) any {
	if member, ok := element.(map[string]any); ok {
		return core.Object(member).Get(name)
	}
	if strings.EqualFold(name, "value") {
		return element
	}
	return nil
}

func inferred(got any) *core.Attribute {
	switch got.(type) {
	case string:
		return inferredString
	case bool:
		return inferredBoolean
	case json.Number, float64, int64, int:
		return inferredDecimal
	}
	return permissiveAttr
}
