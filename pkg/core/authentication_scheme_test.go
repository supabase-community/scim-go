package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthenticationScheme(t *testing.T) {
	t.Run("NewOAuthBearerToken", func(t *testing.T) {
		scheme := NewOAuthBearerToken()

		assert.Equal(t, AuthenticationSchemeOAuthBearerToken, scheme.Type)
		assert.Equal(t, "OAuth Bearer Token", scheme.Name)
		assert.Equal(t, "Authentication scheme using the OAuth Bearer Token Standard", scheme.Description)
		assert.Equal(t, "http://www.rfc-editor.org/info/rfc6750", scheme.SpecURI)
		assert.False(t, scheme.Primary)
	})

	t.Run("AsPrimary marks the scheme primary", func(t *testing.T) {
		scheme := NewOAuthBearerToken()

		require.Same(t, scheme, scheme.AsPrimary())
		assert.True(t, scheme.Primary)
	})
}
