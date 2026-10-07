package server

import (
	"slices"
	"strconv"
	"strings"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// newCharacteristics enforces RFC 7643 Section 2.2, except uniqueness.
func newCharacteristics(schemas core.Schemas) func(existing, candidate core.Object) error {
	fields := newFields(schemas)
	constrained := slices.DeleteFunc(slices.Clone(fields), func(field field) bool { return !isConstrained(field) })
	immutables := slices.DeleteFunc(fields, func(field field) bool { return !isImmutable(field) })
	return func(existing, candidate core.Object) error {
		for _, field := range constrained {
			if err := conforms(field, candidate); err != nil {
				return err
			}
		}
		return immutable(immutables, existing, candidate)
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

func isImmutable(field field) bool {
	return field.Mutability == core.MutabilityImmutable && (field.parent == nil || !field.parent.MultiValued)
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

// immutable rejects a change to an "immutable" attribute once a value has been assigned, per RFC 7644, Section 3.5.1.
func immutable(fields fields, existing, candidate core.Object) error {
	for _, field := range fields {
		assigned := field.value(existing)
		if !value.IsUnassigned(assigned) && !value.Equal(field.Attribute, assigned, field.value(candidate)) {
			return scimerrors.ErrMutability(strconv.Quote(field.Name) + " is immutable")
		}
	}
	return nil
}
