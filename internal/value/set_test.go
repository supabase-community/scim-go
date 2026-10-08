package value_test

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
)

func TestSetContainsAgreesWithContains(t *testing.T) {
	cases := []struct {
		name      string
		attribute *core.Attribute
		elements  []any
		want      any
	}{
		{"a scalar string element, folded", core.NewAttribute("schemas", core.TypeString).AsMultiValued(), []any{"Foo"}, "foo"},
		{"a float64 zero and negative zero", core.NewAttribute("n", core.TypeDecimal).AsMultiValued(), []any{0.0}, math.Copysign(0, -1)},
		{"a complex element without sub-attributes, float64 zero and negative zero", core.NewAttribute("blobs", core.TypeComplex).AsMultiValued(), []any{map[string]any{"n": 0.0}}, map[string]any{"n": math.Copysign(0, -1)}},
		{"a dateTime string by instant regardless of offset", core.NewAttribute("when", core.TypeDateTime).AsMultiValued(), []any{"2026-01-01T12:00:00Z"}, "2026-01-01T13:00:00+01:00"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			set := value.NewSet(c.attribute, c.elements)
			assert.Equal(t, value.Contains(c.attribute, c.elements, c.want), set.Contains(c.want))
		})
	}

	t.Run("a time.Time element by instant regardless of zone", func(t *testing.T) {
		attribute := core.NewAttribute("when", core.TypeDateTime).AsMultiValued()
		instant := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		elements := []any{instant}
		want := instant.In(time.FixedZone("UTC+1", 3600))

		set := value.NewSet(attribute, elements)

		assert.Equal(t, value.Contains(attribute, elements, want), set.Contains(want))
	})
}

func TestSetIdentity(t *testing.T) {
	// RFC 7643 Section 2.2: "caseExact" is "false" unless otherwise stated.
	t.Run("matches a complex element across case", func(t *testing.T) {
		set := value.NewSet(addressesLike(), []any{map[string]any{"postalCode": "aI"}})

		assert.True(t, set.Contains(map[string]any{"postalCode": "AI"}))
		assert.False(t, set.Contains(map[string]any{"postalCode": "bI"}))
	})

	t.Run("does not match a caseExact sub-attribute across case", func(t *testing.T) {
		attribute := core.NewAttribute("parts", core.TypeComplex).AsMultiValued().With(
			core.NewAttribute("serial", core.TypeString).AsCaseExact(),
		)
		set := value.NewSet(attribute, []any{map[string]any{"serial": "aI"}})

		assert.True(t, set.Contains(map[string]any{"serial": "aI"}))
		assert.False(t, set.Contains(map[string]any{"serial": "AI"}))
	})

	t.Run("finds the first stored element", func(t *testing.T) {
		first := core.Object{"value": "a@b.com", "display": "first"}
		set := value.NewSet(emailsLike(), []any{first, core.Object{"value": "A@B.com", "display": "second"}})

		assert.Equal(t, first, set.Find(core.Object{"value": "a@b.com"}))
	})

	t.Run("finds nothing for an element without identity", func(t *testing.T) {
		set := value.NewSet(emailsLike(), []any{core.Object{"type": "work"}})

		assert.Nil(t, set.Find(core.Object{"type": "work"}))
	})
}

func addressesLike() *core.Attribute {
	return core.NewAttribute("addresses", core.TypeComplex).AsMultiValued().With(
		core.NewAttribute("postalCode", core.TypeString),
	)
}
