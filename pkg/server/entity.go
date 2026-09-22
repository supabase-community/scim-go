package server

import "github.com/supabase-community/scim-go/pkg/core"

type Entity interface {
	core.Resource
	SetID(id string)
	GetMeta() core.Meta
	SetMeta(meta core.Meta)
	SetSchemas(schemas []core.SchemaURI)
}
