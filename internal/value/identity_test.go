package value_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
)

func TestIdentity(t *testing.T) {
	// RFC 7643 Section 4.2: sub-attributes of members are "immutable".
	t.Run("ignores type when type is immutable", func(t *testing.T) {
		assert.Equal(t, value.Identity(membersLike(), core.Object{"value": "u-1", "type": "User"}), value.Identity(membersLike(), core.Object{"value": "u-1", "type": "Group"}))
	})

	// RFC 7643 Section 2.4: the same "value" MAY repeat under a different "type".
	t.Run("includes type when type is readWrite", func(t *testing.T) {
		assert.NotEqual(t, value.Identity(emailsLike(), core.Object{"value": "a@b.com", "type": "work"}), value.Identity(emailsLike(), core.Object{"value": "a@b.com", "type": "home"}))
	})

	t.Run("returns empty for a missing value regardless of type", func(t *testing.T) {
		assert.Empty(t, value.Identity(emailsLike(), core.Object{"type": "work"}))
		assert.Empty(t, value.Identity(membersLike(), core.Object{"type": "User"}))
	})

	t.Run("normalizes an unassigned type", func(t *testing.T) {
		assert.Equal(t, value.Identity(emailsLike(), core.Object{"value": "a@b.com"}), value.Identity(emailsLike(), core.Object{"value": "a@b.com", "type": ""}))
	})

	t.Run("folds the case of value and type", func(t *testing.T) {
		assert.Equal(t, value.Identity(emailsLike(), core.Object{"value": "a@b.com", "type": "work"}), value.Identity(emailsLike(), core.Object{"value": "A@B.com", "type": "Work"}))
	})

	t.Run("falls back to the writable sub-attributes when there is no value", func(t *testing.T) {
		attribute := core.NewAttribute("parts", core.TypeComplex).AsMultiValued().With(
			core.NewAttribute("serial", core.TypeString),
			core.NewAttribute("inspector", core.TypeString).AsReadOnly(),
		)
		assert.Equal(t, value.Identity(attribute, core.Object{"serial": "s-1", "inspector": "a"}), value.Identity(attribute, core.Object{"serial": "s-1", "inspector": "b"}))
		assert.NotEqual(t, value.Identity(attribute, core.Object{"serial": "s-1"}), value.Identity(attribute, core.Object{"serial": "s-2"}))
	})

	// RFC 7644 Section 3.5.1: immutable input values MUST match, or HTTP status code 400 SHOULD be returned.
	t.Run("ignores an immutable sub-attribute when there is no value", func(t *testing.T) {
		attribute := core.NewAttribute("parts", core.TypeComplex).AsMultiValued().With(
			core.NewAttribute("serial", core.TypeString),
			core.NewAttribute("code", core.TypeString).AsImmutable(),
		)
		assert.Equal(t, value.Identity(attribute, core.Object{"serial": "s-1", "code": "A"}), value.Identity(attribute, core.Object{"serial": "s-1", "code": "B"}))
	})

	t.Run("compares a decimal sub-attribute by value", func(t *testing.T) {
		attribute := core.NewAttribute("parts", core.TypeComplex).AsMultiValued().With(
			core.NewAttribute("n", core.TypeDecimal),
		)
		for _, pair := range [][2]json.Number{{"0", "-0"}, {"1", "1.0"}, {"1", "1e0"}} {
			assert.Equal(t, value.Identity(attribute, core.Object{"n": pair[0]}), value.Identity(attribute, core.Object{"n": pair[1]}))
		}
		assert.NotEqual(t, value.Identity(attribute, core.Object{"n": json.Number("1")}), value.Identity(attribute, core.Object{"n": json.Number("2")}))
	})

	t.Run("compares a dateTime sub-attribute by instant", func(t *testing.T) {
		attribute := core.NewAttribute("parts", core.TypeComplex).AsMultiValued().With(
			core.NewAttribute("when", core.TypeDateTime),
		)
		assert.Equal(t, value.Identity(attribute, core.Object{"when": "2026-01-01T12:00:00Z"}), value.Identity(attribute, core.Object{"when": "2026-01-01T13:00:00+01:00"}))
	})
}

func emailsLike() *core.Attribute {
	return core.NewAttribute("emails", core.TypeComplex).AsMultiValued().With(
		core.NewAttribute("value", core.TypeString),
		core.NewAttribute("type", core.TypeString),
	)
}

func membersLike() *core.Attribute {
	return core.NewAttribute("members", core.TypeComplex).AsMultiValued().With(
		core.NewAttribute("value", core.TypeString).AsImmutable(),
		core.NewAttribute("type", core.TypeString).AsImmutable(),
	)
}
