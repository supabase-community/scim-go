package scim

import (
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
)

// BasePath is the mount point of the SCIM service, per RFC 7644, Section 3.2.
const BasePath = "/scim/v2"

func Join(base, segment string) string {
	return strings.TrimSuffix(base, "/") + "/" + strings.TrimPrefix(segment, "/")
}

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
