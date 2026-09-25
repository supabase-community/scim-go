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

// RFC 7643 Section 7: "writeOnly" and "returned" "never" attribute values SHALL NOT be returned.
func hidden(attribute *core.Attribute) bool {
	return attribute.Mutability == core.MutabilityWriteOnly || attribute.Returned == core.ReturnedNever
}
