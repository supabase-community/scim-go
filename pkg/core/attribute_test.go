package core_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
)

func TestNewAttribute(t *testing.T) {
	attribute := core.NewAttribute("userName", core.TypeString).DescribedAs("A unique identifier for the user.")

	t.Run("describes the attribute it names", func(t *testing.T) {
		assert.Equal(t, "userName", attribute.Name)
		assert.Equal(t, core.TypeString, attribute.Type)
		assert.Equal(t, "A unique identifier for the user.", attribute.Description)
	})

	t.Run("defaults to a readWrite attribute returned by default", func(t *testing.T) {
		assert.Equal(t, core.MutabilityReadWrite, attribute.Mutability)
		assert.Equal(t, core.ReturnedDefault, attribute.Returned)
		assert.Equal(t, core.UniquenessNone, attribute.Uniqueness)
	})

	t.Run("defaults to an optional, single valued, case insensitive attribute", func(t *testing.T) {
		assert.False(t, attribute.Required)
		assert.False(t, attribute.MultiValued)
		assert.False(t, attribute.CaseExact)
	})

	t.Run("serializes to JSON correctly", func(t *testing.T) {
		body, err := json.Marshal(attribute)

		require.NoError(t, err)
		require.JSONEq(t, `{
			"name": "userName",
			"type": "string",
			"multiValued": false,
			"description": "A unique identifier for the user.",
			"required": false,
			"caseExact": false,
			"mutability": "readWrite",
			"returned": "default",
			"uniqueness": "none"
		}`, string(body))
	})

	t.Run("marks the attribute the client must send", func(t *testing.T) {
		attribute := core.NewAttribute("userName", core.TypeString).DescribedAs("A unique identifier for the user.")

		require.Same(t, attribute, attribute.AsRequired())
		assert.True(t, attribute.Required)
	})

	t.Run("marks the attribute that holds more than one value", func(t *testing.T) {
		attribute := core.NewAttribute("emails", core.TypeComplex).DescribedAs("The email addresses for the user.")

		require.Same(t, attribute, attribute.AsMultiValued())
		assert.True(t, attribute.MultiValued)
	})

	t.Run("marks the attribute whose value is compared case sensitively", func(t *testing.T) {
		attribute := core.NewAttribute("id", core.TypeString).DescribedAs("A unique identifier for the resource.")

		require.Same(t, attribute, attribute.AsCaseExact())
		assert.True(t, attribute.CaseExact)
	})

	t.Run("states the scope the service provider enforces uniqueness over", func(t *testing.T) {
		attribute := core.NewAttribute("userName", core.TypeString).DescribedAs("A unique identifier for the user.")

		require.Same(t, attribute, attribute.UniqueOn(core.UniquenessServer))

		body, err := json.Marshal(attribute)

		require.NoError(t, err)
		require.Contains(t, string(body), `"uniqueness":"server"`)
	})

	t.Run("suggests the canonical values a client may send", func(t *testing.T) {
		attribute := core.NewAttribute("type", core.TypeString).
			DescribedAs("A label indicating the attribute's function.").
			Suggesting("work", "home", "other")

		body, err := json.Marshal(attribute)

		require.NoError(t, err)
		require.Contains(t, string(body), `"canonicalValues":["work","home","other"]`)
	})

	t.Run("names the resource types a reference may point at", func(t *testing.T) {
		attribute := core.NewAttribute("$ref", core.TypeReference).DescribedAs("The URI of the corresponding resource.")

		require.Same(t, attribute, attribute.Referencing(core.ReferenceType("User"), core.ReferenceExternal, core.ReferenceURI))

		body, err := json.Marshal(attribute)

		require.NoError(t, err)
		require.Contains(t, string(body), `"referenceTypes":["User","external","uri"]`)
	})

	t.Run("nests the sub-attributes of a complex attribute", func(t *testing.T) {
		givenName := core.NewAttribute("givenName", core.TypeString).DescribedAs("The given name of the user.")
		name := core.NewAttribute("name", core.TypeComplex).DescribedAs("The components of the user's name.")

		require.Same(t, name, name.With(givenName))
		require.Equal(t, core.Attributes{givenName}, name.SubAttributes)

		body, err := json.Marshal(name)

		require.NoError(t, err)
		require.JSONEq(t, `{
			"name": "name",
			"type": "complex",
			"multiValued": false,
			"description": "The components of the user's name.",
			"required": false,
			"caseExact": false,
			"mutability": "readWrite",
			"returned": "default",
			"uniqueness": "none",
			"subAttributes": [{
				"name": "givenName",
				"type": "string",
				"multiValued": false,
				"description": "The given name of the user.",
				"required": false,
				"caseExact": false,
				"mutability": "readWrite",
				"returned": "default",
				"uniqueness": "none"
			}]
		}`, string(body))
	})

	t.Run("composes every refinement in a chain", func(t *testing.T) {
		attribute := core.NewAttribute("emails", core.TypeComplex).
			DescribedAs("The email addresses for the user.").
			AsRequired().
			AsMultiValued().
			AsCaseExact().
			UniqueOn(core.UniquenessGlobal).
			Suggesting("work", "home").
			With(core.NewAttribute("value", core.TypeString).DescribedAs("The email address."))

		assert.True(t, attribute.Required)
		assert.True(t, attribute.MultiValued)
		assert.True(t, attribute.CaseExact)
		assert.Equal(t, core.UniquenessGlobal, attribute.Uniqueness)
		assert.Equal(t, []string{"work", "home"}, attribute.CanonicalValues)
		assert.Len(t, attribute.SubAttributes, 1)

		body, err := json.Marshal(attribute)

		require.NoError(t, err)
		require.Contains(t, string(body), `"uniqueness":"global"`)
	})

	t.Run("serializes an attribute the client can neither write nor read back", func(t *testing.T) {
		attribute := core.NewAttribute("password", core.TypeString).DescribedAs("The user's cleartext password.")
		attribute.Mutability = core.MutabilityWriteOnly
		attribute.Returned = core.ReturnedNever

		body, err := json.Marshal(attribute)

		require.NoError(t, err)
		require.Contains(t, string(body), `"mutability":"writeOnly"`)
		require.Contains(t, string(body), `"returned":"never"`)
	})

	t.Run("finds a sub-attribute case-insensitively", func(t *testing.T) {
		emails := core.NewAttribute("emails", core.TypeComplex).AsMultiValued().With(
			core.NewAttribute("value", core.TypeString),
		)
		assert.Equal(t, "value", emails.SubAttribute("VALUE").Name)
	})

	t.Run("returns nil for an absent sub-attribute", func(t *testing.T) {
		emails := core.NewAttribute("emails", core.TypeComplex).AsMultiValued().With(
			core.NewAttribute("value", core.TypeString),
		)
		assert.Nil(t, emails.SubAttribute("missing"))
	})

	t.Run("sets mutability to immutable", func(t *testing.T) {
		attribute := core.NewAttribute("id", core.TypeString)
		require.Same(t, attribute, attribute.AsImmutable())
		assert.Equal(t, core.MutabilityImmutable, attribute.Mutability)
	})

	t.Run("sets mutability to writeOnly", func(t *testing.T) {
		attribute := core.NewAttribute("password", core.TypeString)
		require.Same(t, attribute, attribute.AsWriteOnly())
		assert.Equal(t, core.MutabilityWriteOnly, attribute.Mutability)
	})
}

func TestAttributeCoerce(t *testing.T) {
	t.Run("returns matching values as their typed form", func(t *testing.T) {
		s, ok := core.NewAttribute("userName", core.TypeString).Coerce("mo")
		require.True(t, ok)
		assert.Equal(t, "mo", s)

		b, ok := core.NewAttribute("active", core.TypeBoolean).Coerce(true)
		require.True(t, ok)
		assert.Equal(t, true, b)

		d, ok := core.NewAttribute("score", core.TypeDecimal).Coerce(1.5)
		require.True(t, ok)
		assert.InDelta(t, 1.5, d, 0)

		i, ok := core.NewAttribute("count", core.TypeInteger).Coerce(float64(3))
		require.True(t, ok)
		assert.Equal(t, int64(3), i)

		ts, ok := core.NewAttribute("created", core.TypeDateTime).Coerce("2026-09-10T00:00:00Z")
		require.True(t, ok)
		assert.Equal(t, time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC), ts)
	})

	t.Run("passes a null value through unchanged", func(t *testing.T) {
		v, ok := core.NewAttribute("active", core.TypeBoolean).Coerce(nil)
		require.True(t, ok)
		assert.Nil(t, v)
	})

	t.Run("rejects a value whose type does not match", func(t *testing.T) {
		_, ok := core.NewAttribute("active", core.TypeBoolean).Coerce("yes")
		require.False(t, ok)
	})

	t.Run("rejects a non-integral value for an integer attribute", func(t *testing.T) {
		_, ok := core.NewAttribute("count", core.TypeInteger).Coerce(1.5)
		require.False(t, ok)
	})

	t.Run("parses an integer attribute from a json.Number", func(t *testing.T) {
		i, ok := core.NewAttribute("count", core.TypeInteger).Coerce(json.Number("3"))
		require.True(t, ok)
		assert.Equal(t, int64(3), i)
	})

	t.Run("preserves an integer beyond float64 precision", func(t *testing.T) {
		i, ok := core.NewAttribute("id", core.TypeInteger).Coerce(json.Number("9007199254740993"))
		require.True(t, ok)
		assert.Equal(t, int64(9007199254740993), i)
	})

	t.Run("rejects a non-integral json.Number for an integer attribute", func(t *testing.T) {
		_, ok := core.NewAttribute("count", core.TypeInteger).Coerce(json.Number("1.5"))
		require.False(t, ok)
	})

	t.Run("parses a decimal attribute from a json.Number", func(t *testing.T) {
		d, ok := core.NewAttribute("score", core.TypeDecimal).Coerce(json.Number("1.5"))
		require.True(t, ok)
		assert.InDelta(t, 1.5, d, 0)
	})

	t.Run("accepts an int64 for an integer attribute", func(t *testing.T) {
		i, ok := core.NewAttribute("count", core.TypeInteger).Coerce(int64(7))
		require.True(t, ok)
		assert.Equal(t, int64(7), i)
	})

	t.Run("rejects a malformed dateTime", func(t *testing.T) {
		_, ok := core.NewAttribute("created", core.TypeDateTime).Coerce("yesterday")
		require.False(t, ok)
	})

	t.Run("rejects a non-string value for a dateTime attribute", func(t *testing.T) {
		_, ok := core.NewAttribute("created", core.TypeDateTime).Coerce(123)
		require.False(t, ok)
	})

	t.Run("accepts a base64 value for a binary attribute", func(t *testing.T) {
		b, ok := core.NewAttribute("cert", core.TypeBinary).Coerce("aGVsbG8=")
		require.True(t, ok)
		assert.Equal(t, "aGVsbG8=", b)
	})

	t.Run("rejects a non-base64 value for a binary attribute", func(t *testing.T) {
		_, ok := core.NewAttribute("cert", core.TypeBinary).Coerce("not base64!")
		require.False(t, ok)
	})

	t.Run("rejects a scalar compared to a complex attribute", func(t *testing.T) {
		_, ok := core.NewAttribute("name", core.TypeComplex).Coerce("mo")
		require.False(t, ok)
	})

	t.Run("rejects a non-string value for a binary attribute", func(t *testing.T) {
		_, ok := core.NewAttribute("cert", core.TypeBinary).Coerce(123)
		require.False(t, ok)
	})

	t.Run("rejects a non-numeric value for a decimal attribute", func(t *testing.T) {
		_, ok := core.NewAttribute("score", core.TypeDecimal).Coerce("high")
		require.False(t, ok)
	})

	t.Run("rejects a non-numeric value for an integer attribute", func(t *testing.T) {
		_, ok := core.NewAttribute("count", core.TypeInteger).Coerce("many")
		require.False(t, ok)
	})
}
