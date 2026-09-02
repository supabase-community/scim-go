package scim

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewServiceProviderConfig(t *testing.T) {
	t.Run("advertises the schemes the caller declares", func(t *testing.T) {
		scheme := NewOAuthBearerToken().AsPrimary()

		config := NewServiceProviderConfig("", scheme)

		require.Equal(t, []SchemaURI{SchemaServiceProviderConfig}, config.Schemas)
		require.Equal(t, []*AuthenticationScheme{scheme}, config.AuthenticationSchemes)
	})

	t.Run("identifies itself with resource metadata", func(t *testing.T) {
		baseURL := "http://example.com/scim/v2"

		config := NewServiceProviderConfig(baseURL)

		require.Equal(t, ResourceTypeName("ServiceProviderConfig"), config.Meta.ResourceType)
		require.Equal(t, baseURL+"/ServiceProviderConfig", config.Meta.Location)
	})

	t.Run("supports none of the optional protocol features", func(t *testing.T) {
		config := NewServiceProviderConfig("")

		assert.False(t, config.Patch.Supported)
		assert.False(t, config.Bulk.Supported)
		assert.False(t, config.Filter.Supported)
		assert.False(t, config.ChangePassword.Supported)
		assert.False(t, config.Sort.Supported)
		assert.False(t, config.ETag.Supported)
	})

	t.Run("Sorting claims support for sortBy and sortOrder", func(t *testing.T) {
		config := NewServiceProviderConfig("").Sorting()

		assert.True(t, config.Sort.Supported)
		assert.False(t, config.Filter.Supported, "claiming one feature claims no other")
	})

	t.Run("Sorting reaches the wire", func(t *testing.T) {
		body, err := json.Marshal(NewServiceProviderConfig("").Sorting())

		require.NoError(t, err)
		require.Contains(t, string(body), `"sort":{"supported":true}`)
	})

	t.Run("serializes authenticationSchemes as an array", func(t *testing.T) {
		body, err := json.Marshal(NewServiceProviderConfig(""))

		require.NoError(t, err)
		require.Contains(t, string(body), `"authenticationSchemes":[]`)
	})
}
