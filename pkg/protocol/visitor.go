package protocol

import (
	"fmt"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type visitor[Output any] struct {
	schemas core.Schemas
	inner   Evaluator[Output]
	scope   *core.Attribute
	inURI   bool
}

func (r *visitor[Output]) VisitEquals(path filter.AttrPath, value any) (Output, error) {
	return r.compare(path, filter.OpEquals, value)
}

func (r *visitor[Output]) VisitNotEquals(path filter.AttrPath, value any) (Output, error) {
	return r.compare(path, filter.OpNotEquals, value)
}

func (r *visitor[Output]) VisitContains(path filter.AttrPath, value any) (Output, error) {
	return r.compare(path, filter.OpContains, value)
}

func (r *visitor[Output]) VisitStartsWith(path filter.AttrPath, value any) (Output, error) {
	return r.compare(path, filter.OpStartsWith, value)
}

func (r *visitor[Output]) VisitEndsWith(path filter.AttrPath, value any) (Output, error) {
	return r.compare(path, filter.OpEndsWith, value)
}

func (r *visitor[Output]) VisitGreaterThan(path filter.AttrPath, value any) (Output, error) {
	return r.compare(path, filter.OpGreaterThan, value)
}

func (r *visitor[Output]) VisitGreaterThanEquals(path filter.AttrPath, value any) (Output, error) {
	return r.compare(path, filter.OpGreaterThanEquals, value)
}

func (r *visitor[Output]) VisitLessThan(path filter.AttrPath, value any) (Output, error) {
	return r.compare(path, filter.OpLessThan, value)
}

func (r *visitor[Output]) VisitLessThanEquals(path filter.AttrPath, value any) (Output, error) {
	return r.compare(path, filter.OpLessThanEquals, value)
}

func (r *visitor[Output]) VisitPresence(path filter.AttrPath) (Output, error) {
	var zero Output
	attribute, err := r.resolve(path)
	if err != nil {
		return zero, err
	}
	if err := r.conceal(path, attribute, "pr"); err != nil {
		return zero, err
	}
	return r.inner.Present(NewAttribute(attribute, path, r.scope))
}

func (r *visitor[Output]) VisitAnd(left, right Output) (Output, error) {
	return r.inner.And(left, right)
}

func (r *visitor[Output]) VisitOr(left, right Output) (Output, error) {
	return r.inner.Or(left, right)
}

func (r *visitor[Output]) VisitNot(operand Output) (Output, error) {
	return r.inner.Not(operand)
}

func (r *visitor[Output]) VisitValuePath(path filter.AttrPath, subAttribute string, valueFilter func() (Output, error)) (Output, error) {
	var zero Output
	attribute, err := r.resolve(path)
	if err != nil {
		return zero, err
	}
	if !attribute.MultiValued || subAttribute != "" {
		return zero, scimerrors.ErrInvalidFilter(fmt.Sprintf("%q is not a value-path target", path.String()))
	}
	return r.inner.ValuePath(NewAttribute(attribute, path, nil), r.scoped(attribute, valueFilter))
}

func (r *visitor[Output]) scoped(attribute *core.Attribute, valueFilter func() (Output, error)) func() (Output, error) {
	return func() (Output, error) {
		previous := r.scope
		r.scope = attribute
		out, err := valueFilter()
		r.scope = previous
		return out, err
	}
}

func (r *visitor[Output]) resolve(path filter.AttrPath) (*core.Attribute, error) {
	if r.scope != nil {
		return r.resolveWithin(path)
	}
	attribute, ok := r.schemas.Resolve(core.SchemaURI(path.URI), path.Name, path.SubAttribute)
	if !ok {
		return nil, scimerrors.ErrInvalidFilter(fmt.Sprintf("%q is not a known attribute", path.String()))
	}
	return attribute, nil
}

func (r *visitor[Output]) parent(path filter.AttrPath) *core.Attribute {
	if r.scope != nil || path.SubAttribute == "" {
		return r.scope
	}
	parent, _ := r.schemas.Resolve(core.SchemaURI(path.URI), path.Name, "")
	return parent
}

func (r *visitor[Output]) resolveWithin(path filter.AttrPath) (*core.Attribute, error) {
	attribute := r.scope.SubAttribute(path.Name)
	if attribute == nil || path.SubAttribute != "" {
		return nil, scimerrors.ErrInvalidFilter(fmt.Sprintf("%q is not a known attribute", path.String()))
	}
	return attribute, nil
}

func (r *visitor[Output]) compare(path filter.AttrPath, op filter.Operator, literal any) (Output, error) {
	var zero Output
	attribute, err := r.resolve(path)
	if err != nil {
		return zero, err
	}
	if err := r.conceal(path, attribute, op); err != nil {
		return zero, err
	}
	if !value.Allowed(attribute.Type, op) {
		return zero, scimerrors.ErrInvalidFilter(fmt.Sprintf("operator %q is not valid for %q", op, path.String()))
	}
	coerced, ok := attribute.Coerce(literal)
	if !ok {
		return zero, scimerrors.ErrInvalidValue(fmt.Sprintf("%q is not a valid value for %q", literal, path.String()))
	}
	return r.inner.Compare(NewAttribute(attribute, path, r.scope), op, coerced)
}

// RFC 7643 Section 4.1.1: a password is used to "compare (i.e., filter for equality)".
func (r *visitor[Output]) conceal(path filter.AttrPath, attribute *core.Attribute, op filter.Operator) error {
	switch {
	case !value.Hidden(r.parent(path), attribute):
		return nil
	case r.inURI:
		// RFC 7644 Section 7.5.2: a GET filter with sensitive information SHOULD be refused with 403.
		return scimerrors.ErrSensitive(fmt.Sprintf("a filter on %q must not be sent in a request URI", path.String()))
	case op != filter.OpEquals:
		return scimerrors.ErrInvalidFilter(fmt.Sprintf("operator %q is not valid for %q", op, path.String()))
	}
	return nil
}
