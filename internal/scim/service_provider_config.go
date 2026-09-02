package scim

import "github.com/supabase-community/go-scim/pkg/core"

func NewServiceProviderConfig(baseURL string, schemes ...*core.AuthenticationScheme) *core.ServiceProviderConfig {
	if schemes == nil {
		schemes = []*core.AuthenticationScheme{}
	}
	return &core.ServiceProviderConfig{
		Schemas:               []core.SchemaURI{core.SchemaServiceProviderConfig},
		AuthenticationSchemes: schemes,
		Meta: core.Meta{
			ResourceType: "ServiceProviderConfig",
			Location:     Join(baseURL, "/ServiceProviderConfig"),
		},
	}
}
