package protocol

import (
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
)

type Attribute struct {
	*core.Attribute
	filter.AttrPath
}

func NewAttribute(attr *core.Attribute, path filter.AttrPath) *Attribute {
	return &Attribute{
		Attribute: attr,
		AttrPath:  path,
	}
}
