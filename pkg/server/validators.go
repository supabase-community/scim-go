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

// characteristics enforces the attribute characteristics of RFC 7643, Section 7, except uniqueness, which the Repository enforces atomically with the write.
func characteristics[T Entity](schemas core.Schemas, repo Repository[T]) Validator[T] {
	return checker[T]{readers: readersOf(schemas), repo: repo}.validate
}

type checker[T Entity] struct {
	readers readers
	repo    Repository[T]
}

func (c checker[T]) validate(ctx context.Context, candidate T) error {
	object, err := core.NewObject(candidate)
	if err != nil {
		return err
	}
	if err := required(c.readers, object); err != nil {
		return err
	}
	if err := canonicalValues(c.readers, object); err != nil {
		return err
	}
	return c.mutability(ctx, candidate, object)
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

// mutability rejects a change to an "immutable" attribute once a value has been assigned, per RFC 7643, Section 7.
func (c checker[T]) mutability(ctx context.Context, candidate T, after core.Object) error {
	if candidate.ResourceID() == "" {
		return nil
	}
	existing, err := c.repo.Get(ctx, candidate.ResourceID())
	if err != nil {
		if errors.Is(err, scimerrors.ErrNotFound("")) {
			return nil
		}
		return err
	}
	attribute, err := changedImmutable(c.readers, existing, after)
	if err != nil || attribute == nil {
		return err
	}
	return scimerrors.ErrMutability(strconv.Quote(attribute.Name) + " is immutable")
}

func changedImmutable(readers readers, existing any, after core.Object) (*core.Attribute, error) {
	before, err := core.NewObject(existing)
	if err != nil {
		return nil, err
	}
	for _, attribute := range readers.immutable {
		read := readers.values[attribute]
		previous := read(before)
		if isEmpty(previous) || reflect.DeepEqual(previous, read(after)) {
			continue
		}
		return attribute, nil
	}
	return nil, nil
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
