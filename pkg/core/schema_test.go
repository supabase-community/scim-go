package core_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
)

func exampleSchema() *core.Schema {
	schema := &core.Schema{ID: core.SchemaUser, Name: "User"}
	return schema.With(
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
		schema := &core.Schema{
			Schemas:     []core.SchemaURI{core.SchemaSchema},
			ID:          core.SchemaUser,
			Name:        "User",
			Description: "User Account",
			Attributes: []*core.Attribute{
				core.NewAttribute("userName", core.TypeString).DescribedAs("A unique identifier for the user.").AsRequired(),
			},
			Meta: core.Meta{
				ResourceType: "Schema",
				Location:     "http://example.com/scim/v2/Schemas/urn:ietf:params:scim:schemas:core:2.0:User",
			},
		}

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
