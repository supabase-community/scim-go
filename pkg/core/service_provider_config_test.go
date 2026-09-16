package core_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
)

func TestNewServiceProviderConfig(t *testing.T) {
	t.Run("supports none of the optional protocol features", func(t *testing.T) {
		config := core.NewServiceProviderConfig()

		assert.False(t, config.Patch.Supported)
		assert.False(t, config.Bulk.Supported)
		assert.False(t, config.Filter.Supported)
		assert.False(t, config.ChangePassword.Supported)
		assert.False(t, config.Sort.Supported)
		assert.False(t, config.ETag.Supported)
	})

	t.Run("the builders announce the features the provider honours", func(t *testing.T) {
		config := core.NewServiceProviderConfig().Patching().Sorting().Filtering(200)

		assert.True(t, config.Patch.Supported)
		assert.True(t, config.Sort.Supported)
		assert.True(t, config.Filter.Supported)
		assert.Equal(t, 200, config.Filter.MaxResults)
	})

	t.Run("serializes to JSON correctly", func(t *testing.T) {
		config := core.NewServiceProviderConfig().Patching().Filtering(200)

		body, err := json.Marshal(config)

		require.NoError(t, err)
		require.JSONEq(t, `{
			"schemas": ["urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"],
			"patch": {"supported": true},
			"bulk": {"supported": false, "maxOperations": 0, "maxPayloadSize": 0},
			"filter": {"supported": true, "maxResults": 200},
			"changePassword": {"supported": false},
			"sort": {"supported": false},
			"etag": {"supported": false},
			"authenticationSchemes": null,
			"meta": {"resourceType": "ServiceProviderConfig"}
		}`, string(body))
	})
}
