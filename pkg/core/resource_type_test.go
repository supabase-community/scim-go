package core_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
)

func TestResourceType(t *testing.T) {
	t.Run("Extend appends schema extensions", func(t *testing.T) {
		resourceType := &core.ResourceType{Name: "User"}

		require.Same(t, resourceType, resourceType.Extend(core.SchemaExtension{
			Schema:   core.SchemaEnterpriseUser,
			Required: true,
		}))

		assert.Equal(t, []core.SchemaExtension{{
			Schema:   core.SchemaEnterpriseUser,
			Required: true,
		}}, resourceType.SchemaExtensions)
	})

	t.Run("serializes to JSON correctly", func(t *testing.T) {
		body, err := json.Marshal(userResourceType())

		require.NoError(t, err)
		require.JSONEq(t, `{
			"schemas": ["urn:ietf:params:scim:schemas:core:2.0:ResourceType"],
			"id": "User",
			"name": "User",
			"endpoint": "/Users",
			"description": "User Account",
			"schema": "urn:ietf:params:scim:schemas:core:2.0:User",
			"schemaExtensions": [
				{"schema": "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User", "required": true}
			],
			"meta": {
				"resourceType": "ResourceType",
				"location": "https://example.com/v2/ResourceTypes/User"
			}
		}`, string(body))
	})
}

func userResourceType() *core.ResourceType {
	return (&core.ResourceType{
		Schemas:     []core.SchemaURI{core.SchemaResourceType},
		ID:          "User",
		Name:        "User",
		Endpoint:    "/Users",
		Description: "User Account",
		Schema:      core.SchemaUser,
		Meta: core.Meta{
			ResourceType: "ResourceType",
			Location:     "https://example.com/v2/ResourceTypes/User",
		},
	}).Extend(core.SchemaExtension{
		Schema:   core.SchemaEnterpriseUser,
		Required: true,
	})
}
