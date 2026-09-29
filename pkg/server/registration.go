package server

import "github.com/supabase-community/scim-go/pkg/core"

type Registration interface {
	resourceType() *core.ResourceType
	schemas(base string) core.Schemas
	mount(s *Server, schemas core.Schemas)
}
