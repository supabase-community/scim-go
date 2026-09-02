package scim

import "github.com/supabase-community/go-scim/pkg/core"

func NewResourceType(baseURL string, kind Kind, schema *core.Schema) *core.ResourceType {
	resourceType := &core.ResourceType{
		Schemas:     []core.SchemaURI{core.SchemaResourceType},
		ID:          kind.Name,
		Name:        kind.Name,
		Description: schema.Description,
		Endpoint:    kind.Endpoint,
		Schema:      schema.ID,
		Meta: core.Meta{
			ResourceType: KindResourceType.Name,
			Location:     kind.Location(baseURL),
		},
	}

	return resourceType
}
