package protocol

import (
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
)

type Attribute struct {
	Definition *core.Attribute
	Path       filter.AttrPath
	Parent     *core.Attribute
}

func NewAttribute(definition *core.Attribute, path filter.AttrPath, parent *core.Attribute) *Attribute {
	return &Attribute{
		Definition: definition,
		Path:       path,
		Parent:     parent,
	}
}
