package core

import (
	"encoding/json"
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

	t.Run("serializes to JSON correctly", func(t *testing.T) {
		scheme := NewOAuthBearerToken().AsPrimary()

		body, err := json.Marshal(scheme)

		require.NoError(t, err)
		require.JSONEq(t, `{
			"type": "oauthbearertoken",
			"name": "OAuth Bearer Token",
			"description": "Authentication scheme using the OAuth Bearer Token Standard",
			"specUri": "http://www.rfc-editor.org/info/rfc6750",
			"primary": true
		}`, string(body))
	})

	t.Run("omits the primary flag when the scheme is not primary", func(t *testing.T) {
		body, err := json.Marshal(NewOAuthBearerToken())

		require.NoError(t, err)
		assert.NotContains(t, string(body), "primary")
	})
}
