package server

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// characteristics enforces RFC 7643 Section 2.2, except uniqueness.
func characteristics[T core.Resource](schemas core.Schemas) Validator[T] {
	fields := newFields(schemas)
	constrained := slices.DeleteFunc(slices.Clone(fields), func(field field) bool { return !isConstrained(field) })
	return func(ctx context.Context, candidate T) error {
		after, ok := candidateFrom(ctx)
		if !ok {
			var err error
			after, err = core.NewObject(candidate)
			if err != nil {
				return err
			}
		}
		for _, field := range constrained {
			if err := conforms(field, after); err != nil {
				return err
			}
		}
		return immutable(fields, previous(ctx, candidate), after)
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

func previous[T core.Resource](ctx context.Context, candidate T) core.Object {
	if candidate.Common().ID == "" {
		return core.Object{}
	}
	before, _ := existingFrom(ctx)
	return before
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
	subs := immutableSubs(field.SubAttributes)
	if !field.MultiValued || len(subs) == 0 {
		return nil
	}
	return immutableElements(field, subs, before, after)
}

func immutableSubs(subs []*core.Attribute) []*core.Attribute {
	return slices.DeleteFunc(slices.Clone(subs), func(sub *core.Attribute) bool { return sub.Mutability != core.MutabilityImmutable })
}

type elementIndex struct {
	signatures map[string]map[string]struct{}
	sample     map[string]core.Object
}

// RFC 7643 Section 4.2: while values MAY be added or removed, sub-attributes of members are "immutable".
func immutableElements(field field, subs []*core.Attribute, before, after core.Object) error {
	index := newElementIndex(field, subs, after)
	sub := changedElement(field, subs, before, index)
	if sub == nil {
		return nil
	}
	return scimerrors.ErrMutability(strconv.Quote(sub.Name) + " is immutable")
}

func newElementIndex(field field, subs []*core.Attribute, after core.Object) elementIndex {
	index := elementIndex{signatures: map[string]map[string]struct{}{}, sample: map[string]core.Object{}}
	for _, element := range field.elements(after) {
		candidate := asObject(element)
		key := value.Identity(field.Attribute, candidate)
		if key == "" {
			continue
		}
		if index.signatures[key] == nil {
			index.signatures[key] = map[string]struct{}{}
			index.sample[key] = candidate
		}
		index.signatures[key][signature(subs, candidate)] = struct{}{}
	}
	return index
}

func changedElement(field field, subs []*core.Attribute, before core.Object, index elementIndex) *core.Attribute {
	for _, element := range field.elements(before) {
		stored := asObject(element)
		key := value.Identity(field.Attribute, stored)
		if key == "" {
			continue
		}
		if _, matched := index.signatures[key][signature(subs, stored)]; matched || index.signatures[key] == nil {
			continue
		}
		if sub := changed(subs, stored, index.sample[key]); sub != nil {
			return sub
		}
	}
	return nil
}

func signature(subs []*core.Attribute, element core.Object) string {
	var b strings.Builder
	for _, sub := range subs {
		b.WriteString(foldedString(sub, coerce(sub, element.Get(sub.Name))))
		b.WriteByte(0)
	}
	return b.String()
}

func foldedString(sub *core.Attribute, v any) string {
	switch folded := value.Fold(sub, v).(type) {
	case string:
		return folded
	case time.Time:
		return folded.UTC().Format(time.RFC3339Nano)
	default:
		return fmt.Sprint(folded)
	}
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
