package protocol

import (
	"fmt"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
)

func Filter[T any](schemas []*core.Schema, text string, v Evaluator[T]) (T, error) {
	var zero T
	node, err := filter.Parse(text)
	if err != nil {
		return zero, ErrInvalidFilter(err.Error())
	}
	return filter.Visit[T](&resolvingVisitor[T]{schemas: schemas, inner: v}, node)
}

type resolvingVisitor[T any] struct {
	schemas []*core.Schema
	inner   Evaluator[T]
	scope   *core.Attribute
}

func (r *resolvingVisitor[T]) VisitEquals(path filter.AttrPath, value any) (T, error) {
	return r.compare(path, filter.OpEquals, value)
}

func (r *resolvingVisitor[T]) VisitNotEquals(path filter.AttrPath, value any) (T, error) {
	return r.compare(path, filter.OpNotEquals, value)
}

func (r *resolvingVisitor[T]) VisitContains(path filter.AttrPath, value any) (T, error) {
	return r.compare(path, filter.OpContains, value)
}

func (r *resolvingVisitor[T]) VisitStartsWith(path filter.AttrPath, value any) (T, error) {
	return r.compare(path, filter.OpStartsWith, value)
}

func (r *resolvingVisitor[T]) VisitEndsWith(path filter.AttrPath, value any) (T, error) {
	return r.compare(path, filter.OpEndsWith, value)
}

func (r *resolvingVisitor[T]) VisitGreaterThan(path filter.AttrPath, value any) (T, error) {
	return r.compare(path, filter.OpGreaterThan, value)
}

func (r *resolvingVisitor[T]) VisitGreaterThanEquals(path filter.AttrPath, value any) (T, error) {
	return r.compare(path, filter.OpGreaterThanEquals, value)
}

func (r *resolvingVisitor[T]) VisitLessThan(path filter.AttrPath, value any) (T, error) {
	return r.compare(path, filter.OpLessThan, value)
}

func (r *resolvingVisitor[T]) VisitLessThanEquals(path filter.AttrPath, value any) (T, error) {
	return r.compare(path, filter.OpLessThanEquals, value)
}

func (r *resolvingVisitor[T]) VisitPresence(path filter.AttrPath) (T, error) {
	var zero T
	attribute, err := r.resolve(path)
	if err != nil {
		return zero, err
	}
	return r.inner.Present(attribute, path.Key())
}

func (r *resolvingVisitor[T]) VisitAnd(left, right T) (T, error) {
	return r.inner.And(left, right)
}

func (r *resolvingVisitor[T]) VisitOr(left, right T) (T, error) {
	return r.inner.Or(left, right)
}

func (r *resolvingVisitor[T]) VisitNot(operand T) (T, error) {
	return r.inner.Not(operand)
}

func (r *resolvingVisitor[T]) VisitValuePath(path filter.AttrPath, subAttribute string, valueFilter func() (T, error)) (T, error) {
	var zero T
	attribute, err := r.resolve(path)
	if err != nil {
		return zero, err
	}
	if !attribute.MultiValued || subAttribute != "" {
		return zero, ErrInvalidFilter(fmt.Sprintf("%q is not a value-path target", path.String()))
	}
	return r.inner.ValuePath(attribute, path.Key(), r.scoped(attribute, valueFilter))
}

func (r *resolvingVisitor[T]) scoped(attribute *core.Attribute, valueFilter func() (T, error)) func() (T, error) {
	return func() (T, error) {
		previous := r.scope
		r.scope = attribute
		out, err := valueFilter()
		r.scope = previous
		return out, err
	}
}

func (r *resolvingVisitor[T]) resolve(path filter.AttrPath) (*core.Attribute, error) {
	if r.scope != nil {
		return r.resolveWithin(path)
	}
	schema, ok := r.selectSchema(path.URI)
	if !ok {
		return nil, ErrInvalidFilter(fmt.Sprintf("%q is not a known attribute", path.String()))
	}
	attribute, ok := schema.Resolve(path.Name)
	if !ok {
		return nil, ErrInvalidFilter(fmt.Sprintf("%q is not a known attribute", path.String()))
	}
	if path.SubAttribute != "" {
		attribute = attribute.SubAttribute(path.SubAttribute)
		if attribute == nil {
			return nil, ErrInvalidFilter(fmt.Sprintf("%q is not a known attribute", path.String()))
		}
	}
	return attribute, nil
}

func (r *resolvingVisitor[T]) resolveWithin(path filter.AttrPath) (*core.Attribute, error) {
	attribute := r.scope.SubAttribute(path.Name)
	if attribute == nil || path.SubAttribute != "" {
		return nil, ErrInvalidFilter(fmt.Sprintf("%q is not a known attribute", path.String()))
	}
	return attribute, nil
}

func (r *resolvingVisitor[T]) selectSchema(uri string) (*core.Schema, bool) {
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

func (r *resolvingVisitor[T]) compare(path filter.AttrPath, op filter.Operator, value any) (T, error) {
	var zero T
	attribute, err := r.resolve(path)
	if err != nil {
		return zero, err
	}
	if !operatorAllowed(attribute.Type, op) {
		return zero, ErrInvalidFilter(fmt.Sprintf("operator %q is not valid for %q", op, path.String()))
	}
	coerced, ok := attribute.Coerce(value)
	if !ok {
		return zero, ErrInvalidValue(fmt.Sprintf("%q is not a valid value for %q", value, path.String()))
	}
	return r.inner.Compare(attribute, path.Key(), op, coerced)
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
