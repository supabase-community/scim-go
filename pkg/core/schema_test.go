package core

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

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
}
