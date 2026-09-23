package server

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// Validators enforces the attribute characteristics of RFC 7643, Section 7, except uniqueness, which the Repository enforces atomically with the write.
func Validators[T Entity](fields Fields[T], repo Repository[T]) []Validator[T] {
	return []Validator[T]{
		Required(fields),
		CanonicalValues(fields),
		Mutability(fields, repo),
	}
}

// Required rejects a candidate missing a value for an attribute marked "required", per RFC 7643, Section 7.
func Required[T Entity](fields Fields[T]) Validator[T] {
	accessors := fields.accessors()
	elements := fields.elements()
	return func(_ context.Context, candidate T) error {
		for attribute, accessor := range accessors {
			if attribute.Required && isMissing(attribute, accessor(candidate)) {
				return scimerrors.ErrInvalidValue(strconv.Quote(attribute.Name) + " is required")
			}
		}
		for attribute, list := range elements {
			if attribute.Required && len(list(candidate)) == 0 {
				return scimerrors.ErrInvalidValue(strconv.Quote(attribute.Name) + " is required")
			}
		}
		return nil
	}
}

func isMissing(attribute *core.Attribute, value any) bool {
	if attribute.MultiValued {
		return isEmpty(value)
	}
	return slices.ContainsFunc(valuesOf(value), isEmpty)
}

// CanonicalValues rejects a value that is not among an attribute's declared "canonicalValues", per RFC 7643, Section 7.
func CanonicalValues[T Entity](fields Fields[T]) Validator[T] {
	accessors := fields.accessors()
	return func(_ context.Context, candidate T) error {
		for attribute, accessor := range accessors {
			if len(attribute.CanonicalValues) == 0 {
				continue
			}
			for _, raw := range valuesOf(accessor(candidate)) {
				value, ok := raw.(string)
				if !ok || value == "" || containsValue(attribute.CanonicalValues, value, attribute.CaseExact) {
					continue
				}
				return scimerrors.ErrInvalidValue(strconv.Quote(value) + " is not a canonical value for " + strconv.Quote(attribute.Name))
			}
		}
		return nil
	}
}

func valuesOf(raw any) []any {
	if list, ok := raw.([]any); ok {
		return list
	}
	return []any{raw}
}

// Mutability rejects a change to an "immutable" attribute once a value has been assigned, per RFC 7643, Section 7.
func Mutability[T Entity](fields Fields[T], repo Repository[T]) Validator[T] {
	accessors := fields.accessors()
	return func(ctx context.Context, candidate T) error {
		if candidate.ResourceID() == "" {
			return nil
		}
		existing, err := repo.Get(ctx, candidate.ResourceID())
		if err != nil {
			if errors.Is(err, scimerrors.ErrNotFound("")) {
				return nil
			}
			return err
		}
		if attribute := changedImmutable(accessors, existing, candidate); attribute != nil {
			return scimerrors.ErrMutability(strconv.Quote(attribute.Name) + " is immutable")
		}
		return nil
	}
}

func changedImmutable[T Entity](accessors accessorSet[T], existing, candidate T) *core.Attribute {
	for attribute, accessor := range accessors {
		if attribute.Mutability != core.MutabilityImmutable {
			continue
		}
		previous := accessor(existing)
		if isEmpty(previous) || reflect.DeepEqual(previous, accessor(candidate)) {
			continue
		}
		return attribute
	}
	return nil
}

func isEmpty(value any) bool {
	if value == nil {
		return true
	}
	switch v := reflect.ValueOf(value); v.Kind() {
	case reflect.String, reflect.Slice, reflect.Map, reflect.Array:
		return v.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return v.IsNil()
	default:
		return false
	}
}

func containsValue(values []string, value string, caseExact bool) bool {
	return slices.ContainsFunc(values, func(candidate string) bool {
		return sameValue(candidate, value, caseExact)
	})
}

func sameValue(a, b any, caseExact bool) bool {
	as, aIsString := a.(string)
	bs, bIsString := b.(string)
	if aIsString && bIsString && !caseExact {
		return strings.EqualFold(as, bs)
	}
	return reflect.DeepEqual(a, b)
}
