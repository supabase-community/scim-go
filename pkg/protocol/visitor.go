package protocol

import (
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

func newVisitor[Output any](schemas core.Schemas, inner Evaluator[Output], inURI bool) *visitor[Output] {
	return &visitor[Output]{schemas: schemas, inner: inner, inURI: inURI}
}

func (r *visitor[Output]) VisitEquals(path filter.AttrPath, literal any) (Output, error) {
	return r.compare(path, filter.OpEquals, literal)
}

func (r *visitor[Output]) VisitNotEquals(path filter.AttrPath, literal any) (Output, error) {
	return r.compare(path, filter.OpNotEquals, literal)
}

func (r *visitor[Output]) VisitContains(path filter.AttrPath, literal any) (Output, error) {
	return r.compare(path, filter.OpContains, literal)
}

func (r *visitor[Output]) VisitStartsWith(path filter.AttrPath, literal any) (Output, error) {
	return r.compare(path, filter.OpStartsWith, literal)
}

func (r *visitor[Output]) VisitEndsWith(path filter.AttrPath, literal any) (Output, error) {
	return r.compare(path, filter.OpEndsWith, literal)
}

func (r *visitor[Output]) VisitGreaterThan(path filter.AttrPath, literal any) (Output, error) {
	return r.compare(path, filter.OpGreaterThan, literal)
}

func (r *visitor[Output]) VisitGreaterThanEquals(path filter.AttrPath, literal any) (Output, error) {
	return r.compare(path, filter.OpGreaterThanEquals, literal)
}

func (r *visitor[Output]) VisitLessThan(path filter.AttrPath, literal any) (Output, error) {
	return r.compare(path, filter.OpLessThan, literal)
}

func (r *visitor[Output]) VisitLessThanEquals(path filter.AttrPath, literal any) (Output, error) {
	return r.compare(path, filter.OpLessThanEquals, literal)
}

func (r *visitor[Output]) VisitPresence(path filter.AttrPath) (Output, error) {
	var zero Output
	attribute, err := r.resolve(path)
	if err != nil {
		return zero, err
	}
	if err := r.conceal(path, r.parent(path), attribute, filter.OpPresent); err != nil {
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

func (r *visitor[Output]) VisitValuePath(path filter.AttrPath, valueFilter func() (Output, error)) (Output, error) {
	var zero Output
	attribute, err := r.resolve(path)
	if err != nil {
		return zero, err
	}
	if !attribute.MultiValued {
		return zero, invalidFilter()
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
		return nil, invalidFilter()
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
		return nil, invalidFilter()
	}
	return attribute, nil
}

func (r *visitor[Output]) compare(path filter.AttrPath, op filter.Operator, literal any) (Output, error) {
	var zero Output
	attribute, err := r.resolve(path)
	if err != nil {
		return zero, err
	}
	parent := r.parent(path)
	// RFC 7644 Section 3.4.2.2: Figure 2 filters a multi-valued attribute without a sub-attribute, e.g. emails co "example.com".
	if path.SubAttribute == "" && attribute.Type == core.TypeComplex && attribute.MultiValued {
		if sub := attribute.SubAttribute("value"); sub != nil {
			parent, attribute = attribute, sub
		}
	}
	if err := r.conceal(path, parent, attribute, op); err != nil {
		return zero, err
	}
	if !value.Allowed(attribute.Type, op) {
		return zero, invalidFilter()
	}
	coerced, ok := attribute.Coerce(literal)
	if !ok {
		return zero, scimerrors.ErrInvalidValue(scimerrors.InvalidValue.Description())
	}
	return r.inner.Compare(NewAttribute(attribute, path, r.scope), op, coerced)
}

// RFC 7643 Section 4.1.1: a password is used to "compare (i.e., filter for equality)".
func (r *visitor[Output]) conceal(path filter.AttrPath, parent, attribute *core.Attribute, op filter.Operator) error {
	switch {
	case !value.Hidden(parent, attribute):
		return nil
	case r.inURI && op != filter.OpPresent:
		// RFC 7644 Section 7.5.2: a GET filter that contains sensitive information SHOULD be refused with 403.
		return scimerrors.ErrSensitive(scimerrors.Sensitive.Description())
	case op != filter.OpEquals:
		return invalidFilter()
	}
	return nil
}

func invalidFilter() error {
	return scimerrors.ErrInvalidFilter(scimerrors.InvalidFilter.Description())
}
