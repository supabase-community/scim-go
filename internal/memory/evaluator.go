package memory

import (
	"slices"

	"github.com/supabase-community/scim-go/internal/fields"
	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
	"github.com/supabase-community/scim-go/pkg/store"
)

type predicate func(row any) bool

type evaluator struct {
	fields.Fields
}

func newEvaluator(all fields.Fields, schemas core.Schemas) protocol.Evaluator[predicate] {
	return &evaluator{Fields: withoutPassword(all, schemas.Base())}
}

// RFC 7643 Section 4.1.1: a held password SHOULD be hashed, so it cannot be compared.
func withoutPassword(all fields.Fields, base *core.Schema) fields.Fields {
	return slices.DeleteFunc(slices.Clone(all), func(f fields.Field) bool {
		return f.Name == store.PasswordAttribute && slices.Contains(base.Attributes, f.Attribute)
	})
}

func (e *evaluator) Compare(attribute *protocol.Attribute, op filter.Operator, literal any) (predicate, error) {
	read, err := e.reader(attribute)
	if err != nil {
		return nil, err
	}
	definition := attribute.Definition
	want := value.Fold(definition, literal)
	return func(row any) bool {
		return anyMatch(read(row), func(raw any) bool {
			return value.Match(op, value.Observed(definition, raw), want)
		})
	}, nil
}

func (e *evaluator) Present(attribute *protocol.Attribute) (predicate, error) {
	read, err := e.reader(attribute)
	if err != nil {
		return nil, err
	}
	return func(row any) bool {
		return anyMatch(read(row), func(v any) bool { return !value.IsUnassigned(v) })
	}, nil
}

func (e *evaluator) And(left, right predicate) (predicate, error) {
	return func(row any) bool { return left(row) && right(row) }, nil
}

func (e *evaluator) Or(left, right predicate) (predicate, error) {
	return func(row any) bool { return left(row) || right(row) }, nil
}

func (e *evaluator) Not(operand predicate) (predicate, error) {
	return func(row any) bool { return !operand(row) }, nil
}

func (e *evaluator) ValuePath(attribute *protocol.Attribute, valueFilter func() (predicate, error)) (predicate, error) {
	list, ok := e.Lookup(attribute.Definition)
	if !ok || !list.IsList() {
		return nil, scimerrors.ErrInvalidFilter(scimerrors.InvalidFilter.Description())
	}
	inner, err := valueFilter()
	if err != nil {
		return nil, err
	}
	return func(row any) bool {
		return slices.ContainsFunc(list.Elements(value.AsObject(row)), inner)
	}, nil
}

func (e *evaluator) reader(attribute *protocol.Attribute) (func(row any) any, error) {
	definition := attribute.Definition
	if attribute.Parent != nil {
		return func(row any) any { return fields.Coerce(definition, value.AsObject(row).Get(definition.Name)) }, nil
	}
	if field, ok := e.Lookup(definition); ok {
		return func(row any) any { return field.Value(value.AsObject(row)) }, nil
	}
	return nil, scimerrors.ErrInvalidFilter(scimerrors.InvalidFilter.Description())
}

// RFC 7644 3.4.2.2 - a multi-valued attribute matches if any value does, for every operator including ne.
func anyMatch(raw any, pred func(any) bool) bool {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 {
		return pred(raw)
	}
	return slices.ContainsFunc(list, pred)
}
