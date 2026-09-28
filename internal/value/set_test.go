package value_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
)

func TestSetContains(t *testing.T) {
	t.Run("finds a complex element equal by every sub-attribute", func(t *testing.T) {
		set := value.NewSet(addressesLike(), []any{map[string]any{"postalCode": "aI"}})

		assert.True(t, set.Contains(map[string]any{"postalCode": "aI"}))
		assert.False(t, set.Contains(map[string]any{"postalCode": "bI"}))
	})

	// RFC 7643 Section 2.3.6: "postalCode" is not caseExact.
	t.Run("folds case for a complex element's string sub-attribute", func(t *testing.T) {
		set := value.NewSet(addressesLike(), []any{map[string]any{"postalCode": "aI"}})

		assert.False(t, set.Contains(map[string]any{"postalCode": "AI"}))
	})

	t.Run("folds a scalar string element", func(t *testing.T) {
		set := value.NewSet(core.NewAttribute("schemas", core.TypeString).AsMultiValued(), []any{"Foo"})

		assert.True(t, set.Contains("foo"))
	})

	t.Run("matches a time.Time element by instant regardless of zone", func(t *testing.T) {
		attribute := core.NewAttribute("when", core.TypeDateTime).AsMultiValued()
		instant := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
		set := value.NewSet(attribute, []any{instant})

		assert.True(t, set.Contains(instant.In(time.FixedZone("UTC+1", 3600))))
	})
}

func addressesLike() *core.Attribute {
	return core.NewAttribute("addresses", core.TypeComplex).AsMultiValued().With(
		core.NewAttribute("postalCode", core.TypeString),
	)
}
