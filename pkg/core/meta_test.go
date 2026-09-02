package core

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/gofrs/uuid"
	"github.com/stretchr/testify/require"
)

type exampleUser struct {
	id               string
	created, updated time.Time
}

func (s exampleUser) ResourceID() string { return s.id }

func TestMeta(t *testing.T) {
	baseURL := "https://example.com/scim/v2"

	t.Run("NewMeta", func(t *testing.T) {
		meta := NewMeta(baseURL, KindServiceProviderConfig)

		require.Equal(t, KindServiceProviderConfig.Name, meta.ResourceType)
		require.Equal(t, baseURL+"/ServiceProviderConfig", meta.Location)
		require.Zero(t, meta.Created)
		require.Zero(t, meta.LastModified)
	})

	t.Run("For", func(t *testing.T) {
		t.Run("locates the resource under its collection", func(t *testing.T) {
			resource := exampleUser{
				id: uuid.Must(uuid.NewV4()).String(),
			}

			meta := NewMeta(baseURL, KindUser).For(resource)

			require.Equal(t, KindUser.Name, meta.ResourceType)
			require.Equal(t, baseURL+"/Users/"+resource.id, meta.Location)
		})
	})

	t.Run("json.Marshal", func(t *testing.T) {
		t.Run("serializes the minimal representation", func(t *testing.T) {
			meta := Meta{ResourceType: "Example"}

			body, err := json.Marshal(meta)

			require.NoError(t, err)
			require.JSONEq(t, `{"resourceType": "Example"}`, string(body))
		})

		t.Run("serializes the full representation", func(t *testing.T) {
			createdAt := time.Date(2026, 7, 21, 19, 41, 41, 0, time.UTC)
			updatedAt := time.Date(2026, 7, 22, 8, 12, 3, 0, time.UTC)
			meta := Meta{
				ResourceType: "User",
				Created:      createdAt,
				LastModified: updatedAt,
				Location:     "http://example.com/scim/v2/Users/2819c223-7f76-453a-919d-413861904646",
				Version:      "etag",
			}

			body, err := json.Marshal(meta)

			require.NoError(t, err)
			require.JSONEq(t, `{
				"resourceType": "User",
				"created": "2026-07-21T19:41:41Z",
				"lastModified": "2026-07-22T08:12:03Z",
				"location": "http://example.com/scim/v2/Users/2819c223-7f76-453a-919d-413861904646",
				"version": "etag"
			}`, string(body))
		})
	})
}
