package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/supabase-community/go-scim/pkg/scimtest"
)

func TestNewServiceProviderConfig(t *testing.T) {
	t.Run("supports none of the optional protocol features", func(t *testing.T) {
		config := &ServiceProviderConfig{}

		assert.False(t, config.Patch.Supported)
		assert.False(t, config.Bulk.Supported)
		assert.False(t, config.Filter.Supported)
		assert.False(t, config.ChangePassword.Supported)
		assert.False(t, config.Sort.Supported)
		assert.False(t, config.ETag.Supported)
	})

	t.Run("json.Marshal", func(t *testing.T) {
		config := &ServiceProviderConfig{
			Schemas:               []SchemaURI{SchemaServiceProviderConfig},
			AuthenticationSchemes: []*AuthenticationScheme{NewOAuthBearerToken().AsPrimary()},
		}

		scimtest.AssertJSON(t, scimtest.RFC7643ServiceProviderConfiguration, config)
	})
}
