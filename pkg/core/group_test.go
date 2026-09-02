package core

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGroup(t *testing.T) {
	created := time.Date(2010, 1, 23, 4, 56, 22, 0, time.UTC)
	lastModified := time.Date(2011, 05, 13, 4, 42, 34, 0, time.UTC)

	group := Group{
		Schemas:     []SchemaURI{SchemaGroup},
		ID:          "e9e30dba-f08f-4109-8486-d5c6a331660a",
		DisplayName: "Tour Guides",
		Members: []Member{
			{
				Value:   "2819c223-7f76-453a-919d-413861904646",
				Ref:     "https://example.com/v2/Users/2819c223-7f76-453a-919d-413861904646",
				Display: "Babs Jensen",
			},
		},
		Meta: Meta{
			ResourceType: KindGroup.Name,
			Created:      created,
			LastModified: lastModified,
			Location:     "http://example.com/scim/v2/Groups/e9e30dba-f08f-4109-8486-d5c6a331660a",
		},
	}

	t.Run("serializes to JSON", func(t *testing.T) {
		body, err := json.Marshal(group)

		require.NoError(t, err)
		require.JSONEq(t, `{
			"schemas": ["urn:ietf:params:scim:schemas:core:2.0:Group"],
			"id": "e9e30dba-f08f-4109-8486-d5c6a331660a",
			"displayName": "Tour Guides",
			"members": [
				{
					"value": "2819c223-7f76-453a-919d-413861904646",
					"$ref": "https://example.com/v2/Users/2819c223-7f76-453a-919d-413861904646",
					"display": "Babs Jensen"
				}
			],
			"meta": {
				"resourceType": "Group",
				"created": "2010-01-23T04:56:22Z",
				"lastModified": "2011-05-13T04:42:34Z",
				"location": "http://example.com/scim/v2/Groups/e9e30dba-f08f-4109-8486-d5c6a331660a"
			}
		}`, string(body))
	})
}
