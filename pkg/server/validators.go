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

// characteristics enforces the attribute characteristics of RFC 7643, Section 2.2, except uniqueness, which the Repository enforces atomically with the write.
func characteristics[T Entity](schemas core.Schemas, repo Repository[T]) Validator[T] {
	readers := readersOf(schemas)
	return func(ctx context.Context, candidate T) error {
		after, err := core.NewObject(candidate)
		if err != nil {
			return err
		}
		if err := required(readers, after); err != nil {
			return err
		}
		if err := canonicalValues(readers, after); err != nil {
			return err
		}
		before, err := previous(ctx, repo, candidate)
		if err != nil {
			return err
		}
		return immutable(readers, before, after)
	}
}

// required rejects a candidate missing a value for an attribute marked "required", per RFC 7643, Section 7.
func required(readers readers, candidate core.Object) error {
	for attribute, read := range readers.values {
		if attribute.Required && isMissing(attribute, read(candidate)) {
			return scimerrors.ErrInvalidValue(strconv.Quote(attribute.Name) + " is required")
		}
	}
	return nil
}

func isMissing(attribute *core.Attribute, value any) bool {
	if attribute.MultiValued {
		return isEmpty(value)
	}
	return slices.ContainsFunc(valuesOf(value), isEmpty)
}

// canonicalValues rejects a value that is not among an attribute's declared "canonicalValues", per RFC 7643, Section 7.
func canonicalValues(readers readers, candidate core.Object) error {
	for attribute, read := range readers.values {
		if len(attribute.CanonicalValues) == 0 {
			continue
		}
		for _, raw := range valuesOf(read(candidate)) {
			value, ok := raw.(string)
			if !ok || value == "" || containsValue(attribute.CanonicalValues, value, attribute.CaseExact) {
				continue
			}
			return scimerrors.ErrInvalidValue(strconv.Quote(value) + " is not a canonical value for " + strconv.Quote(attribute.Name))
		}
	}
	return nil
}

func valuesOf(raw any) []any {
	if list, ok := raw.([]any); ok {
		return list
	}
	return []any{raw}
}

func previous[T Entity](ctx context.Context, repo Repository[T], candidate T) (core.Object, error) {
	if candidate.ResourceID() == "" {
		return core.Object{}, nil
	}
	existing, err := repo.Get(ctx, candidate.ResourceID())
	if errors.Is(err, scimerrors.ErrNotFound("")) {
		return core.Object{}, nil
	}
	if err != nil {
		return nil, err
	}
	return core.NewObject(existing)
}

// immutable rejects a change to an "immutable" attribute once a value has been assigned, per RFC 7643, Section 7.
func immutable(readers readers, before, after core.Object) error {
	for _, attribute := range readers.immutable {
		read := readers.values[attribute]
		assigned := read(before)
		if isEmpty(assigned) || reflect.DeepEqual(assigned, read(after)) {
			continue
		}
		return scimerrors.ErrMutability(strconv.Quote(attribute.Name) + " is immutable")
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
