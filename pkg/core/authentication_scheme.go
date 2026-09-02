package core

type AuthenticationSchemeType string

// The authentication scheme types of RFC 7643, Section 5.
const (
	AuthenticationSchemeOAuth            AuthenticationSchemeType = "oauth"
	AuthenticationSchemeOAuth2           AuthenticationSchemeType = "oauth2"
	AuthenticationSchemeOAuthBearerToken AuthenticationSchemeType = "oauthbearertoken"
	AuthenticationSchemeHTTPBasic        AuthenticationSchemeType = "httpbasic"
	AuthenticationSchemeHTTPDigest       AuthenticationSchemeType = "httpdigest"
)

type AuthenticationScheme struct {
	Type             AuthenticationSchemeType `json:"type"`
	Name             string                   `json:"name"`
	Description      string                   `json:"description"`
	SpecURI          string                   `json:"specUri,omitempty"`
	DocumentationURI string                   `json:"documentationUri,omitempty"`
	Primary          bool                     `json:"primary,omitempty"`
}

func NewOAuthBearerToken() *AuthenticationScheme {
	return &AuthenticationScheme{
		Type:        AuthenticationSchemeOAuthBearerToken,
		Name:        "OAuth Bearer Token",
		Description: "Authentication scheme using the OAuth Bearer Token Standard",
		SpecURI:     "http://www.rfc-editor.org/info/rfc6750",
	}
}

func (scheme *AuthenticationScheme) AsPrimary() *AuthenticationScheme {
	scheme.Primary = true
	return scheme
}
