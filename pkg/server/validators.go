package server

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// characteristics enforces the attribute characteristics of RFC 7643, Section 2.2, except uniqueness, which the Repository enforces atomically with the write.
func characteristics[T core.Resource](schemas core.Schemas, repo Repository[T]) Validator[T] {
	fields := newFields(schemas)
	constrained := slices.DeleteFunc(slices.Clone(fields), func(field field) bool { return !isConstrained(field) })
	return func(ctx context.Context, candidate T) error {
		after, err := core.NewObject(candidate)
		if err != nil {
			return err
		}
		for _, field := range constrained {
			if err := conforms(field, after); err != nil {
				return err
			}
		}
		before, err := previous(ctx, repo, candidate)
		if err != nil {
			return err
		}
		return immutable(fields, before, after)
	}
}

// conforms rejects a missing "required" value or a value outside "canonicalValues" (RFC 7643, Section 7), or more than one "primary" (Section 2.4).
func conforms(field field, candidate core.Object) error {
	raw := field.value(candidate)
	values := valuesOf(raw)
	if field.Required && isMissing(field.Attribute, raw) {
		return scimerrors.ErrInvalidValue(strconv.Quote(field.Name) + " is required")
	}
	if isPrimary(field) && count(values, true) > 1 {
		return scimerrors.ErrInvalidValue(`"primary" may be true for at most one value`)
	}
	for _, v := range values {
		s, ok := v.(string)
		if ok && s != "" && len(field.CanonicalValues) > 0 && !containsValue(field.Attribute, field.CanonicalValues, s) {
			return scimerrors.ErrInvalidValue(strconv.Quote(s) + " is not a canonical value for " + strconv.Quote(field.Name))
		}
	}
	return nil
}

func isConstrained(field field) bool {
	return field.Required || isPrimary(field) || len(field.CanonicalValues) > 0
}

func isPrimary(field field) bool {
	return strings.EqualFold(field.Name, "primary")
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
func immutable(fields fields, before, after core.Object) error {
	for _, field := range fields {
		if field.parent != nil && field.parent.MultiValued {
			continue
		}
		assigned := field.value(before)
		if field.Mutability == core.MutabilityImmutable && !value.IsUnassigned(assigned) && !value.Equal(field.Attribute, assigned, field.value(after)) {
			return scimerrors.ErrMutability(strconv.Quote(field.Name) + " is immutable")
		}
		if !field.MultiValued || field.SubAttribute("value") == nil {
			continue
		}
		if err := immutableElements(field.elements, immutableSubs(field.SubAttributes), before, after); err != nil {
			return err
		}
	}
	return nil
}

func immutableSubs(subs []*core.Attribute) []*core.Attribute {
	return slices.DeleteFunc(slices.Clone(subs), func(sub *core.Attribute) bool { return sub.Mutability != core.MutabilityImmutable })
}

// RFC 7643 Section 4.2: while values MAY be added or removed, sub-attributes of members are "immutable".
func immutableElements(elements func(core.Object) []any, subs []*core.Attribute, before, after core.Object) error {
	current := map[string][]core.Object{}
	for _, element := range elements(after) {
		if key, ok := value.Key(asObject(element)); ok {
			current[key] = append(current[key], asObject(element))
		}
	}
	for _, element := range elements(before) {
		key, ok := value.Key(asObject(element))
		candidates := current[key]
		if !ok || len(candidates) == 0 {
			continue
		}
		stored := asObject(element)
		if slices.ContainsFunc(candidates, func(candidate core.Object) bool { return changed(subs, stored, candidate) == nil }) {
			continue
		}
		return scimerrors.ErrMutability(strconv.Quote(changed(subs, stored, candidates[0]).Name) + " is immutable")
	}
	return nil
}

func changed(subs []*core.Attribute, stored, candidate core.Object) *core.Attribute {
	for _, sub := range subs {
		assigned := coerce(sub, stored.Get(sub.Name))
		if !value.IsUnassigned(assigned) && !value.Equal(sub, assigned, coerce(sub, candidate.Get(sub.Name))) {
			return sub
		}
	}
	return nil
}

func containsValue(attribute *core.Attribute, values []string, want string) bool {
	return slices.ContainsFunc(values, func(candidate string) bool {
		return value.Equal(attribute, candidate, want)
	})
}
