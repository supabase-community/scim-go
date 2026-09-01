package core

import "strings"

type Kind struct {
	Name     ResourceTypeName
	Schema   SchemaURI
	Endpoint string
}

var (
	KindGroup                 = Kind{"Group", SchemaGroup, "/Groups"}
	KindResourceType          = Kind{"ResourceType", SchemaResourceType, "/ResourceTypes"}
	KindSchema                = Kind{"Schema", SchemaSchema, "/Schemas"}
	KindServiceProviderConfig = Kind{"ServiceProviderConfig", SchemaServiceProviderConfig, "/ServiceProviderConfig"}
	KindUser                  = Kind{"User", SchemaUser, "/Users"}
)

func (k Kind) Location(baseURL string) string {
	return Join(baseURL, k.Endpoint)
}

func Join(base, segment string) string {
	return strings.TrimSuffix(base, "/") + "/" + strings.TrimPrefix(segment, "/")
}
