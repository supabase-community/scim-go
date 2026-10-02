package server

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// characteristics enforces RFC 7643 Section 2.2, except uniqueness.
func characteristics[T core.Resource](schemas core.Schemas) Validator[T] {
	fields := newFields(schemas)
	constrained := slices.DeleteFunc(slices.Clone(fields), func(field field) bool { return !isConstrained(field) })
	return func(ctx context.Context, _ T) error {
		after, _ := candidateFrom(ctx)
		before, _ := existingFrom(ctx)
		for _, field := range constrained {
			if err := conforms(field, after); err != nil {
				return err
			}
		}
		return immutable(fields, before, after)
	}
}

// conforms rejects a missing "required" value or a value outside "canonicalValues" (RFC 7643, Section 7), more than one "primary" (Section 2.4), or a binary value that is not base64 (Section 2.3.6).
func conforms(field field, candidate core.Object) error {
	raw := field.value(candidate)
	values := valuesOf(raw)
	if field.Required && isMissing(field.Attribute, raw) {
		return scimerrors.ErrInvalidValue(strconv.Quote(field.Name) + " is required")
	}
	if isPrimaryField(field.Attribute) && count(values, true) > 1 {
		return scimerrors.ErrInvalidValue(`"primary" may be true for at most one value`)
	}
	if field.Type == core.TypeBinary && !isEncoded(field, candidate) {
		return scimerrors.ErrInvalidValue(strconv.Quote(field.Name) + " must be base64 encoded")
	}
	if !isCanonical(field, values) {
		return scimerrors.ErrInvalidValue(scimerrors.InvalidValue.Description())
	}
	return nil
}

func isConstrained(field field) bool {
	return field.Required || isPrimaryField(field.Attribute) || len(field.CanonicalValues) > 0 || field.Type == core.TypeBinary
}

func isPrimaryField(attribute *core.Attribute) bool {
	return strings.EqualFold(attribute.Name, "primary")
}

func isCanonical(field field, values []any) bool {
	return len(field.CanonicalValues) == 0 || !slices.ContainsFunc(values, func(v any) bool {
		s, ok := v.(string)
		return ok && s != "" && !value.Contains(field.Attribute, field.CanonicalValues, s)
	})
}

func isEncoded(field field, candidate core.Object) bool {
	return !slices.ContainsFunc(valuesOf(field.raw(candidate)), func(v any) bool {
		_, ok := field.Coerce(v)
		return !ok
	})
}

func isMissing(attribute *core.Attribute, raw any) bool {
	if attribute.MultiValued {
		return value.IsUnassigned(raw)
	}
	return slices.ContainsFunc(valuesOf(raw), value.IsUnassigned)
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

type existingKey struct{}

func withExisting(ctx context.Context, existing core.Object) context.Context {
	return context.WithValue(ctx, existingKey{}, existing)
}

func existingFrom(ctx context.Context) (core.Object, bool) {
	existing, ok := ctx.Value(existingKey{}).(core.Object)
	return existing, ok
}

type candidateKey struct{}

func withCandidate(ctx context.Context, after core.Object) context.Context {
	return context.WithValue(ctx, candidateKey{}, after)
}

func candidateFrom(ctx context.Context) (core.Object, bool) {
	after, ok := ctx.Value(candidateKey{}).(core.Object)
	return after, ok
}

// immutable rejects a change to an "immutable" attribute once a value has been assigned, per RFC 7644, Section 3.5.1.
func immutable(fields fields, before, after core.Object) error {
	for _, field := range fields {
		if err := immutableField(field, before, after); err != nil {
			return err
		}
	}
	return nil
}

func immutableField(field field, before, after core.Object) error {
	if field.parent != nil && field.parent.MultiValued {
		return nil
	}
	assigned := field.value(before)
	if field.Mutability == core.MutabilityImmutable && !value.IsUnassigned(assigned) && !value.Equal(field.Attribute, assigned, field.value(after)) {
		return scimerrors.ErrMutability(strconv.Quote(field.Name) + " is immutable")
	}
	return nil
}
