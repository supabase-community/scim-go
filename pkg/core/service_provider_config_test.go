package core_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
)

const basePath = "/scim/v2"

func TestNewServiceProviderConfig(t *testing.T) {
	t.Run("supports none of the optional protocol features", func(t *testing.T) {
		config := core.NewServiceProviderConfig(basePath)

		assert.False(t, config.Patch.Supported)
		assert.False(t, config.Bulk.Supported)
		assert.False(t, config.Filter.Supported)
		assert.False(t, config.ChangePassword.Supported)
		assert.False(t, config.Sort.Supported)
		assert.False(t, config.ETag.Supported)
		assert.False(t, config.SupportsPatch())
		assert.False(t, config.SupportsFilter())
		assert.False(t, config.SupportsSort())
		assert.False(t, config.SupportsVersioning())
	})

	t.Run("advertises its own base path and location", func(t *testing.T) {
		config := core.NewServiceProviderConfig(basePath)

		assert.Equal(t, basePath, config.BasePath())
		assert.Equal(t, basePath+"/ServiceProviderConfig", config.Meta.Location)
	})

	t.Run("the builders announce the features the provider honours", func(t *testing.T) {
		config := core.NewServiceProviderConfig(basePath).Patching().Sorting().Filtering(200).Versioning()

		assert.True(t, config.Patch.Supported)
		assert.True(t, config.Sort.Supported)
		assert.True(t, config.Filter.Supported)
		assert.Equal(t, 200, config.Filter.MaxResults)
		assert.True(t, config.ETag.Supported)
		assert.True(t, config.SupportsPatch())
		assert.True(t, config.SupportsSort())
		assert.True(t, config.SupportsFilter())
		assert.True(t, config.SupportsVersioning())
	})

	t.Run("serializes to JSON correctly", func(t *testing.T) {
		config := core.NewServiceProviderConfig(basePath).Patching().Filtering(200)

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
			"authenticationSchemes": [],
			"meta": {"resourceType": "ServiceProviderConfig", "location": "/scim/v2/ServiceProviderConfig"}
		}`, string(body))
	})

	t.Run("Authentication appends the given schemes", func(t *testing.T) {
		config := core.NewServiceProviderConfig(basePath).Authentication(core.NewOAuthBearerToken().AsPrimary())

		require.Len(t, config.AuthenticationSchemes, 1)
		assert.Equal(t, core.AuthenticationSchemeOAuthBearerToken, config.AuthenticationSchemes[0].Type)
		assert.True(t, config.AuthenticationSchemes[0].Primary)
	})
}
