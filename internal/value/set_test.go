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
		{"a complex element equal by every sub-attribute", addressesLike(), []any{map[string]any{"postalCode": "aI"}}, map[string]any{"postalCode": "aI"}},
		{"a complex element that differs", addressesLike(), []any{map[string]any{"postalCode": "aI"}}, map[string]any{"postalCode": "bI"}},
		{"a complex element's string sub-attribute case, unfolded like DeepEqual", addressesLike(), []any{map[string]any{"postalCode": "aI"}}, map[string]any{"postalCode": "AI"}},
		{"a scalar string element, folded", core.NewAttribute("schemas", core.TypeString).AsMultiValued(), []any{"Foo"}, "foo"},
		{"a float64 zero and negative zero", core.NewAttribute("n", core.TypeDecimal).AsMultiValued(), []any{0.0}, math.Copysign(0, -1)},
		{"a complex element's float64 zero and negative zero sub-attribute", measurementsLike(), []any{map[string]any{"n": 0.0}}, map[string]any{"n": math.Copysign(0, -1)}},
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

func addressesLike() *core.Attribute {
	return core.NewAttribute("addresses", core.TypeComplex).AsMultiValued().With(
		core.NewAttribute("postalCode", core.TypeString),
	)
}

func measurementsLike() *core.Attribute {
	return core.NewAttribute("measurements", core.TypeComplex).AsMultiValued().With(
		core.NewAttribute("n", core.TypeDecimal),
	)
}
