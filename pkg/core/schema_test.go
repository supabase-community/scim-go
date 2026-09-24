package core_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
)

func exampleSchema() *core.Schema {
	return core.NewSchema(core.SchemaUser).WithName("User").With(
		core.NewAttribute("userName", core.TypeString).AsRequired(),
		core.NewAttribute("name", core.TypeComplex).With(
			core.NewAttribute("familyName", core.TypeString),
		),
		core.NewAttribute("emails", core.TypeComplex).AsMultiValued().With(
			core.NewAttribute("value", core.TypeString),
			core.NewAttribute("primary", core.TypeBoolean),
		),
	)
}

func TestSchema(t *testing.T) {
	t.Run("serializes to JSON correctly", func(t *testing.T) {
		schema := core.NewSchema(core.SchemaUser).
			WithName("User").
			WithLocation("http://example.com/scim/v2/Schemas/urn:ietf:params:scim:schemas:core:2.0:User").
			WithDescription("User Account").
			With(core.NewAttribute("userName", core.TypeString).DescribedAs("A unique identifier for the user.").AsRequired())

		body, err := json.Marshal(schema)

		require.NoError(t, err)
		require.JSONEq(t, `{
			"schemas": ["urn:ietf:params:scim:schemas:core:2.0:Schema"],
			"id": "urn:ietf:params:scim:schemas:core:2.0:User",
			"name": "User",
			"description": "User Account",
			"attributes": [{
				"name": "userName",
				"type": "string",
				"multiValued": false,
				"description": "A unique identifier for the user.",
				"required": true,
				"caseExact": false,
				"mutability": "readWrite",
				"returned": "default",
				"uniqueness": "none"
			}],
			"meta": {
				"resourceType": "Schema",
				"location": "http://example.com/scim/v2/Schemas/urn:ietf:params:scim:schemas:core:2.0:User"
			}
		}`, string(body))
	})

	t.Run("resolves a top-level attribute", func(t *testing.T) {
		attribute, ok := exampleSchema().Resolve("userName")
		require.True(t, ok)
		assert.Equal(t, core.TypeString, attribute.Type)
		assert.False(t, attribute.CaseExact)
	})

	t.Run("matches attribute names case-insensitively", func(t *testing.T) {
		attribute, ok := exampleSchema().Resolve("USERNAME")
		require.True(t, ok)
		assert.Equal(t, "userName", attribute.Name)
	})

	t.Run("resolves common attributes absent from the schema", func(t *testing.T) {
		external, ok := exampleSchema().Resolve("externalId")
		require.True(t, ok)
		assert.True(t, external.CaseExact)

		meta, ok := exampleSchema().Resolve("meta")
		require.True(t, ok)
		assert.Equal(t, core.TypeDateTime, meta.SubAttribute("lastModified").Type)
	})

	t.Run("common attributes resolve even without a schema", func(t *testing.T) {
		var schema *core.Schema
		attribute, ok := schema.Resolve("id")
		require.True(t, ok)
		assert.Equal(t, core.TypeString, attribute.Type)
	})

	t.Run("returns false for an unknown attribute", func(t *testing.T) {
		_, ok := exampleSchema().Resolve("nickName")
		require.False(t, ok)
	})

	t.Run("resolves common attributes with their fixed mutability", func(t *testing.T) {
		schema := exampleSchema()

		id, ok := schema.Resolve("id")
		require.True(t, ok)
		assert.Equal(t, core.MutabilityReadOnly, id.Mutability)
		assert.Equal(t, core.ReturnedAlways, id.Returned)

		meta, ok := schema.Resolve("meta")
		require.True(t, ok)
		assert.Equal(t, core.MutabilityReadOnly, meta.Mutability)
		assert.Equal(t, core.MutabilityReadOnly, meta.SubAttribute("location").Mutability)

		ext, ok := schema.Resolve("externalId")
		require.True(t, ok)
		assert.Equal(t, core.MutabilityReadWrite, ext.Mutability)
	})
}

func TestSchemas(t *testing.T) {
	enterprise := core.NewSchema(core.SchemaEnterpriseUser).With(
		core.NewAttribute("department", core.TypeString),
		core.NewAttribute("manager", core.TypeComplex).With(
			core.NewAttribute("value", core.TypeString),
		),
	)
	schemas := core.Schemas{exampleSchema(), enterprise}

	t.Run("an empty URI selects the base schema", func(t *testing.T) {
		assert.Equal(t, core.SchemaUser, schemas.Lookup("").ID)
		assert.Equal(t, core.SchemaUser, schemas.Base().ID)
	})

	// RFC 7644 Section 3.10: every facet of an attribute path, including the URN, is case insensitive.
	t.Run("selects a schema by URI case-insensitively", func(t *testing.T) {
		assert.Equal(t, enterprise, schemas.Lookup("URN:IETF:params:scim:schemas:extension:enterprise:2.0:user"))
	})

	t.Run("returns nil for an unknown URI", func(t *testing.T) {
		assert.Nil(t, schemas.Lookup("urn:unknown"))
		assert.Nil(t, core.Schemas{}.Lookup(""))
		assert.Nil(t, core.Schemas{}.Base())
	})

	t.Run("lists the extensions", func(t *testing.T) {
		assert.Equal(t, core.Schemas{enterprise}, schemas.Extensions())
		assert.Empty(t, core.Schemas{}.Extensions())
	})

	t.Run("reports whether a schema is an extension", func(t *testing.T) {
		assert.True(t, schemas.IsExtension(enterprise))
		assert.False(t, schemas.IsExtension(schemas.Base()))
		assert.False(t, schemas.IsExtension(nil))
	})

	t.Run("resolves an unqualified attribute against the base schema", func(t *testing.T) {
		attribute, ok := schemas.Resolve("", "USERNAME", "")
		require.True(t, ok)
		assert.Equal(t, "userName", attribute.Name)
	})

	t.Run("resolves a sub-attribute", func(t *testing.T) {
		attribute, ok := schemas.Resolve("", "name", "FAMILYNAME")
		require.True(t, ok)
		assert.Equal(t, "familyName", attribute.Name)
	})

	t.Run("resolves an extension attribute", func(t *testing.T) {
		attribute, ok := schemas.Resolve(core.SchemaEnterpriseUser, "manager", "value")
		require.True(t, ok)
		assert.Equal(t, "value", attribute.Name)
	})

	t.Run("resolves common attributes only through the base schema", func(t *testing.T) {
		_, ok := schemas.Resolve(core.SchemaUser, "id", "")
		assert.True(t, ok)
		_, ok = schemas.Resolve(core.SchemaEnterpriseUser, "id", "")
		assert.False(t, ok)
	})

	t.Run("rejects an unknown schema, attribute, or sub-attribute", func(t *testing.T) {
		_, ok := schemas.Resolve("urn:unknown", "userName", "")
		assert.False(t, ok)
		_, ok = schemas.Resolve("", "bogus", "")
		assert.False(t, ok)
		_, ok = schemas.Resolve("", "name", "bogus")
		assert.False(t, ok)
	})
}
