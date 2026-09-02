package scim

import (
	"strings"

	"github.com/supabase-community/go-scim/pkg/core"
)

// TODO:: remove this
type Kind struct {
	Name     core.ResourceTypeName
	Schema   core.SchemaURI
	Endpoint string
}

var (
	KindGroup = Kind{
		Name:     "Group",
		Schema:   core.SchemaGroup,
		Endpoint: "/Groups",
	}
	KindResourceType = Kind{
		Name:     "ResourceType",
		Schema:   core.SchemaResourceType,
		Endpoint: "/ResourceTypes",
	}
	KindSchema = Kind{
		Name:     "Schema",
		Schema:   core.SchemaSchema,
		Endpoint: "/Schemas",
	}
	KindServiceProviderConfig = Kind{
		Name:     "ServiceProviderConfig",
		Schema:   core.SchemaServiceProviderConfig,
		Endpoint: "/ServiceProviderConfig",
	}
	KindUser = Kind{
		Name:     "User",
		Schema:   core.SchemaUser,
		Endpoint: "/Users",
	}
)

func (k Kind) Location(baseURL string) string {
	return Join(baseURL, k.Endpoint)
}

// TODO:: find a better home for this.
func Join(base, segment string) string {
	return strings.TrimSuffix(base, "/") + "/" + strings.TrimPrefix(segment, "/")
}
