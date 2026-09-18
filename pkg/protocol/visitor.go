package protocol

import (
	"fmt"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type visitor[T any] struct {
	schemas []*core.Schema
	inner   Evaluator[T]
	scope   *core.Attribute
}

func (r *visitor[T]) VisitEquals(path filter.AttrPath, value any) (T, error) {
	return r.compare(path, filter.OpEquals, value)
}

func (r *visitor[T]) VisitNotEquals(path filter.AttrPath, value any) (T, error) {
	return r.compare(path, filter.OpNotEquals, value)
}

func (r *visitor[T]) VisitContains(path filter.AttrPath, value any) (T, error) {
	return r.compare(path, filter.OpContains, value)
}

func (r *visitor[T]) VisitStartsWith(path filter.AttrPath, value any) (T, error) {
	return r.compare(path, filter.OpStartsWith, value)
}

func (r *visitor[T]) VisitEndsWith(path filter.AttrPath, value any) (T, error) {
	return r.compare(path, filter.OpEndsWith, value)
}

func (r *visitor[T]) VisitGreaterThan(path filter.AttrPath, value any) (T, error) {
	return r.compare(path, filter.OpGreaterThan, value)
}

func (r *visitor[T]) VisitGreaterThanEquals(path filter.AttrPath, value any) (T, error) {
	return r.compare(path, filter.OpGreaterThanEquals, value)
}

func (r *visitor[T]) VisitLessThan(path filter.AttrPath, value any) (T, error) {
	return r.compare(path, filter.OpLessThan, value)
}

func (r *visitor[T]) VisitLessThanEquals(path filter.AttrPath, value any) (T, error) {
	return r.compare(path, filter.OpLessThanEquals, value)
}

func (r *visitor[T]) VisitPresence(path filter.AttrPath) (T, error) {
	var zero T
	attribute, err := r.resolve(path)
	if err != nil {
		return zero, err
	}
	return r.inner.Present(NewAttribute(attribute, path))
}

func (r *visitor[T]) VisitAnd(left, right T) (T, error) {
	return r.inner.And(left, right)
}

func (r *visitor[T]) VisitOr(left, right T) (T, error) {
	return r.inner.Or(left, right)
}

func (r *visitor[T]) VisitNot(operand T) (T, error) {
	return r.inner.Not(operand)
}

func (r *visitor[T]) VisitValuePath(path filter.AttrPath, subAttribute string, valueFilter func() (T, error)) (T, error) {
	var zero T
	attribute, err := r.resolve(path)
	if err != nil {
		return zero, err
	}
	if !attribute.MultiValued || subAttribute != "" {
		return zero, scimerrors.ErrInvalidFilter(fmt.Sprintf("%q is not a value-path target", path.String()))
	}
	return r.inner.ValuePath(NewAttribute(attribute, path), r.scoped(attribute, valueFilter))
}

func (r *visitor[T]) scoped(attribute *core.Attribute, valueFilter func() (T, error)) func() (T, error) {
	return func() (T, error) {
		previous := r.scope
		r.scope = attribute
		out, err := valueFilter()
		r.scope = previous
		return out, err
	}
}

func (r *visitor[T]) resolve(path filter.AttrPath) (*core.Attribute, error) {
	if r.scope != nil {
		return r.resolveWithin(path)
	}
	schema, ok := r.selectSchema(path.URI)
	if !ok {
		return nil, scimerrors.ErrInvalidFilter(fmt.Sprintf("%q is not a known attribute", path.String()))
	}
	attribute, ok := schema.Resolve(path.Name)
	if !ok {
		return nil, scimerrors.ErrInvalidFilter(fmt.Sprintf("%q is not a known attribute", path.String()))
	}
	if path.SubAttribute != "" {
		attribute = attribute.SubAttribute(path.SubAttribute)
		if attribute == nil {
			return nil, scimerrors.ErrInvalidFilter(fmt.Sprintf("%q is not a known attribute", path.String()))
		}
	}
	return attribute, nil
}

func (r *visitor[T]) resolveWithin(path filter.AttrPath) (*core.Attribute, error) {
	attribute := r.scope.SubAttribute(path.Name)
	if attribute == nil || path.SubAttribute != "" {
		return nil, scimerrors.ErrInvalidFilter(fmt.Sprintf("%q is not a known attribute", path.String()))
	}
	return attribute, nil
}

func (r *visitor[T]) selectSchema(uri string) (*core.Schema, bool) {
	if uri == "" {
		if len(r.schemas) == 0 {
			return nil, false
		}
		return r.schemas[0], true
	}
	for _, schema := range r.schemas {
		if string(schema.ID) == uri {
			return schema, true
		}
	}
	return nil, false
}

func (r *visitor[T]) compare(path filter.AttrPath, op filter.Operator, value any) (T, error) {
	var zero T
	attribute, err := r.resolve(path)
	if err != nil {
		return zero, err
	}
	if !operatorAllowed(attribute.Type, op) {
		return zero, scimerrors.ErrInvalidFilter(fmt.Sprintf("operator %q is not valid for %q", op, path.String()))
	}
	coerced, ok := attribute.Coerce(value)
	if !ok {
		return zero, scimerrors.ErrInvalidValue(fmt.Sprintf("%q is not a valid value for %q", value, path.String()))
	}
	return r.inner.Compare(NewAttribute(attribute, path), op, coerced)
}

func operatorAllowed(attributeType core.AttributeType, op filter.Operator) bool {
	switch op {
	case filter.OpEquals, filter.OpNotEquals:
		return true
	case filter.OpContains, filter.OpStartsWith, filter.OpEndsWith:
		return attributeType == core.TypeString || attributeType == core.TypeReference
	case filter.OpGreaterThan, filter.OpGreaterThanEquals, filter.OpLessThan, filter.OpLessThanEquals:
		return attributeType == core.TypeString ||
			attributeType == core.TypeDateTime ||
			attributeType == core.TypeInteger ||
			attributeType == core.TypeDecimal
	}
	return false
}
