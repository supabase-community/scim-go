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
	baseURL := "http://example.com/scim/v2"

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
		t.Run("serializes to JSON correctly", func(t *testing.T) {
			body, err := json.Marshal(NewMeta(baseURL, KindServiceProviderConfig))

			require.NoError(t, err)
			require.JSONEq(t, `{
				"resourceType": "ServiceProviderConfig",
				"location": "http://example.com/scim/v2/ServiceProviderConfig"
			}`, string(body))
		})

		t.Run("omits the location when it is empty", func(t *testing.T) {
			body, err := json.Marshal(Meta{ResourceType: "ServiceProviderConfig"})

			require.NoError(t, err)
			require.JSONEq(t, `{"resourceType": "ServiceProviderConfig"}`, string(body))
		})
	})
}
