package server

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// characteristics enforces the attribute characteristics of RFC 7643, Section 2.2, except uniqueness, which the Repository enforces atomically with the write.
func characteristics[T core.Resource](schemas core.Schemas, repo Repository[T]) Validator[T] {
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
		if err := primary(readers, after); err != nil {
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
	for attribute, read := range readers.all() {
		if attribute.Required && isMissing(attribute, read(candidate)) {
			return scimerrors.ErrInvalidValue(strconv.Quote(attribute.Name) + " is required")
		}
	}
	return nil
}

func isMissing(attribute *core.Attribute, raw any) bool {
	if attribute.MultiValued {
		return value.IsUnassigned(raw)
	}
	return slices.ContainsFunc(valuesOf(raw), value.IsUnassigned)
}

// canonicalValues rejects a value that is not among an attribute's declared "canonicalValues", per RFC 7643, Section 7.
func canonicalValues(readers readers, candidate core.Object) error {
	for attribute, read := range readers.all() {
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

// primary rejects more than one value with "primary" set to true, per RFC 7643, Section 2.4.
func primary(readers readers, candidate core.Object) error {
	for attribute, read := range readers.all() {
		if strings.EqualFold(attribute.Name, "primary") && count(valuesOf(read(candidate)), true) > 1 {
			return scimerrors.ErrInvalidValue(`"primary" may be true for at most one value`)
		}
	}
	return nil
}

func count(values []any, target any) int {
	n := 0
	for _, value := range values {
		if value == target {
			n++
		}
	}
	return n
}

func valuesOf(raw any) []any {
	if list, ok := raw.([]any); ok {
		return list
	}
	return []any{raw}
}

func previous[T core.Resource](ctx context.Context, repo Repository[T], candidate T) (core.Object, error) {
	id := candidate.Common().ID
	if id == "" {
		return core.Object{}, nil
	}
	existing, err := repo.Get(ctx, id)
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
		if value.IsUnassigned(assigned) || reflect.DeepEqual(assigned, read(after)) {
			continue
		}
		return scimerrors.ErrMutability(strconv.Quote(attribute.Name) + " is immutable")
	}
	return nil
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
