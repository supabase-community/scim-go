package scim

import "github.com/supabase-community/go-scim/pkg/core"

func NewSchema(baseURL string, kind Kind) *core.Schema {
	schema := &core.Schema{
		Schemas: []core.SchemaURI{core.SchemaSchema},
		ID:      kind.Schema,
		Name:    kind.Name,
		Meta: core.Meta{
			ResourceType: KindSchema.Name,
			Location:     KindSchema.Location(baseURL),
		},
	}
	return schema
}
