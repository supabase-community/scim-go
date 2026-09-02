package scim

import "github.com/supabase-community/go-scim/pkg/core"

func NewMeta(baseURL string, kind Kind) core.Meta {
	return core.Meta{
		ResourceType: kind.Name,
		Location:     kind.Location(baseURL),
	}
}
