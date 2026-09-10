package core

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func exampleSchema() *Schema {
	schema := &Schema{ID: SchemaUser, Name: "User"}
	return schema.With(
		NewAttribute("userName", TypeString, "").AsRequired(),
		NewAttribute("name", TypeComplex, "").With(
			NewAttribute("familyName", TypeString, ""),
		),
		NewAttribute("emails", TypeComplex, "").AsMultiValued().With(
			NewAttribute("value", TypeString, ""),
			NewAttribute("primary", TypeBoolean, ""),
		),
	)
}

func TestSchema(t *testing.T) {
	t.Run("serializes to JSON correctly", func(t *testing.T) {
		schema := &Schema{
			Schemas:     []SchemaURI{SchemaSchema},
			ID:          SchemaUser,
			Name:        "User",
			Description: "User Account",
			Attributes: []*Attribute{
				NewAttribute("userName", TypeString, "A unique identifier for the user.").AsRequired(),
			},
			Meta: Meta{
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
		assert.Equal(t, TypeString, attribute.Type)
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
		assert.Equal(t, TypeDateTime, meta.SubAttribute("lastModified").Type)
	})

	t.Run("common attributes resolve even without a schema", func(t *testing.T) {
		var schema *Schema
		attribute, ok := schema.Resolve("id")
		require.True(t, ok)
		assert.Equal(t, TypeString, attribute.Type)
	})

	t.Run("returns false for an unknown attribute", func(t *testing.T) {
		_, ok := exampleSchema().Resolve("nickName")
		require.False(t, ok)
	})

	t.Run("resolves common attributes with their fixed mutability", func(t *testing.T) {
		schema := exampleSchema()

		id, ok := schema.Resolve("id")
		require.True(t, ok)
		assert.Equal(t, MutabilityReadOnly, id.Mutability)
		assert.Equal(t, ReturnedAlways, id.Returned)

		meta, ok := schema.Resolve("meta")
		require.True(t, ok)
		assert.Equal(t, MutabilityReadOnly, meta.Mutability)
		assert.Equal(t, MutabilityReadOnly, meta.SubAttribute("location").Mutability)

		ext, ok := schema.Resolve("externalId")
		require.True(t, ok)
		assert.Equal(t, MutabilityReadWrite, ext.Mutability)
	})
}
