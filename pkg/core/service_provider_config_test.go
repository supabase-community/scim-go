package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
}
