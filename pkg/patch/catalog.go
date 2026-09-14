package patch

import (
	"strconv"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

var permissiveAttr = &core.Attribute{}

type catalog struct {
	schemas []*core.Schema
}

func (c catalog) attrFor(path filter.Path) (*core.Attribute, error) {
	if len(c.schemas) == 0 {
		return permissiveAttr, nil
	}
	return c.resolveAttr(path)
}

func (c catalog) parentAttr(path filter.Path) (*core.Attribute, error) {
	base := path
	base.SubAttribute = ""
	return c.attrFor(base)
}

func (c catalog) topLevelPath(name string) filter.Path {
	var path filter.Path
	path.Name = name
	return path
}

func (c catalog) subAttr(attr *core.Attribute, name string) *core.Attribute {
	if sub := attr.SubAttribute(name); sub != nil {
		return sub
	}
	return permissiveAttr
}

func (c catalog) resolveAttr(path filter.Path) (*core.Attribute, error) {
	schema := c.selectSchema(path.URI)
	if schema == nil {
		return nil, scimerrors.ErrInvalidPath(strconv.Quote(path.URI) + " is not a known schema")
	}
	attr, ok := schema.Resolve(path.Name)
	if !ok {
		return nil, scimerrors.ErrInvalidPath(strconv.Quote(path.Name) + " is not a known attribute")
	}
	if path.SubAttribute == "" {
		return attr, nil
	}
	sub := attr.SubAttribute(path.SubAttribute)
	if sub == nil {
		return nil, scimerrors.ErrInvalidPath(strconv.Quote(path.SubAttribute) + " is not a known attribute")
	}
	return sub, nil
}

func (c catalog) selectSchema(uri string) *core.Schema {
	if uri == "" {
		if len(c.schemas) == 0 {
			return nil
		}
		return c.schemas[0]
	}
	for _, schema := range c.schemas {
		if string(schema.ID) == uri {
			return schema
		}
	}
	return nil
}
