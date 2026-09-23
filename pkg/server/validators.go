package server

import (
	"context"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// Required rejects a candidate missing a value for an attribute marked "required", per RFC 7643, Section 7.
func Required[T Entity](accessors Accessors[T]) Validator[T] {
	return func(_ context.Context, candidate T) error {
		for attribute, accessor := range accessors {
			if attribute.Required && isMissing(attribute, accessor(candidate)) {
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
func CanonicalValues[T Entity](accessors Accessors[T]) Validator[T] {
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
func Mutability[T Entity](accessors Accessors[T], repo Repository[T]) Validator[T] {
	return func(ctx context.Context, candidate T) error {
		existing, err := repo.Get(ctx, candidate.ResourceID())
		if err != nil {
			return nil
		}
		for attribute, accessor := range accessors {
			if attribute.Mutability != core.MutabilityImmutable {
				continue
			}
			previous := accessor(existing)
			if isEmpty(previous) || reflect.DeepEqual(previous, accessor(candidate)) {
				continue
			}
			return scimerrors.ErrMutability(strconv.Quote(attribute.Name) + " is immutable")
		}
		return nil
	}
}

// Uniqueness rejects a value already used by another resource when an attribute requires "server" or "global" uniqueness, per RFC 7643, Section 7.
func Uniqueness[T Entity](accessors Accessors[T], repo Repository[T]) Validator[T] {
	return func(ctx context.Context, candidate T) error {
		items, _, err := repo.List(ctx, &protocol.SearchRequest{Count: 1 << 30})
		if err != nil {
			return err
		}
		for attribute, accessor := range accessors {
			if attribute.Uniqueness == core.UniquenessNone {
				continue
			}
			if collidesWithAnother(candidate, items, accessor, attribute.CaseExact) {
				return scimerrors.ErrUniqueness(strconv.Quote(attribute.Name) + " must be unique")
			}
		}
		return nil
	}
}

func collidesWithAnother[T Entity](candidate T, items []T, accessor Accessor[T], caseExact bool) bool {
	value := accessor(candidate)
	if isEmpty(value) {
		return false
	}
	for _, item := range items {
		if item.ResourceID() != candidate.ResourceID() && sameValue(value, accessor(item), caseExact) {
			return true
		}
	}
	return false
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
