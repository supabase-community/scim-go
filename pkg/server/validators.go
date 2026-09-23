package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// Validators enforces the attribute characteristics of RFC 7643, Section 7.
func Validators[T Entity](fields Fields[T], repo Repository[T]) []Validator[T] {
	return []Validator[T]{
		Required(fields),
		CanonicalValues(fields),
		Mutability(fields, repo),
		Uniqueness(fields, repo),
	}
}

// Required rejects a candidate missing a value for an attribute marked "required", per RFC 7643, Section 7.
func Required[T Entity](fields Fields[T]) Validator[T] {
	accessors := fields.accessors()
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
		existing, err := repo.Get(ctx, candidate.ResourceID())
		if err != nil {
			if errors.Is(err, scimerrors.ErrNotFound("")) {
				return nil
			}
			return err
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
func Uniqueness[T Entity](fields Fields[T], repo Repository[T]) Validator[T] {
	accessors := fields.accessors()
	paths := fields.paths()
	return func(ctx context.Context, candidate T) error {
		for attribute, accessor := range accessors {
			if attribute.Uniqueness == core.UniquenessNone {
				continue
			}
			query := uniqueQuery(paths[attribute], accessor(candidate))
			if query == "" {
				continue
			}
			items, _, err := repo.List(ctx, &protocol.SearchRequest{Filter: query, Count: 2})
			if err != nil {
				return err
			}
			if slices.ContainsFunc(items, func(item T) bool { return item.ResourceID() != candidate.ResourceID() }) {
				return scimerrors.ErrUniqueness(strconv.Quote(attribute.Name) + " must be unique")
			}
		}
		return nil
	}
}

// RFC 7644 Section 3.4.2.2: a filter that matches any resource holding one of the values.
func uniqueQuery(path string, value any) string {
	var terms []string
	for _, item := range valuesOf(value) {
		if isEmpty(item) {
			continue
		}
		literal, ok := filterLiteral(item)
		if ok {
			terms = append(terms, path+" eq "+literal)
		}
	}
	return strings.Join(terms, " or ")
}

func filterLiteral(value any) (string, bool) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", false
	}
	literal := strings.TrimSpace(buffer.String())
	return literal, literal[0] != '{' && literal[0] != '['
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
