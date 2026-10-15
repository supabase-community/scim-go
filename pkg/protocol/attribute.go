package protocol

import (
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
)

type Attribute struct {
	Definition *core.Attribute
	Path       filter.AttrPath
	Parent     *core.Attribute
	schemas    core.Schemas
}

func newAttribute(schemas core.Schemas, definition *core.Attribute, path filter.AttrPath, parent *core.Attribute) *Attribute {
	return &Attribute{
		Definition: definition,
		Path:       path,
		Parent:     parent,
		schemas:    schemas,
	}
}

func (a *Attribute) Root() *core.Attribute {
	if a.Parent != nil {
		return a.Parent
	}
	if root, ok := a.schemas.Resolve(core.SchemaURI(a.Path.URI), a.Path.Name, ""); ok {
		return root
	}
	return a.Definition
}

func (a *Attribute) InExtension() bool {
	return a.extension(a.Root()) != nil
}

func (a *Attribute) Keys() []string {
	if a.Parent != nil {
		return []string{a.Definition.Name}
	}
	keys := []string{a.Definition.Name}
	root := a.Root()
	if root != a.Definition {
		keys = []string{root.Name, a.Definition.Name}
	}
	if extension := a.extension(root); extension != nil {
		return append([]string{string(extension.ID)}, keys...)
	}
	return keys
}

func (a *Attribute) extension(root *core.Attribute) *core.Schema {
	for _, schema := range a.schemas.Extensions() {
		if schema.Attributes.Lookup(root.Name) == root {
			return schema
		}
	}
	return nil
}
