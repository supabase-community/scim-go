package scimtest

import (
	"maps"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
)

type readOnly struct {
	extension core.SchemaURI
	attribute *core.Attribute
	sub       *core.Attribute
}

func readOnlyIn(t *testing.T, r *resource) []readOnly {
	t.Helper()

	found := []readOnly{}
	for _, schema := range r.schemas {
		extension := core.SchemaURI("")
		if r.schemas.IsExtension(schema) {
			extension = schema.ID
		}
		for _, attribute := range schema.Attributes {
			found = append(found, readOnlyWithin(extension, attribute)...)
		}
	}
	if len(found) == 0 {
		t.Skipf("n/a: %s has no readOnly attribute other than id", r.kind.Name)
	}
	return found
}

func (o readOnly) path() string {
	path := filter.AttrPath{URI: string(o.extension), Name: o.attribute.Name}
	if o.sub != nil {
		path.SubAttribute = o.sub.Name
	}
	return path.String()
}

func (o readOnly) set(body core.Object, marker string) {
	holder := body
	if o.extension != "" {
		holder = core.Object(objectOf(body.Get(string(o.extension))))
		if holder == nil {
			holder = core.Object{}
		}
		body.Set(string(o.extension), map[string]any(holder))
	}
	holder.Set(o.attribute.Name, o.value(holder.Get(o.attribute.Name), marker))
}

func (o readOnly) value(current any, marker string) any {
	if o.sub == nil {
		return readOnlyValue(o.attribute, marker)
	}
	if !o.attribute.MultiValued {
		return o.marked(objectOf(current), marker)
	}
	var first map[string]any
	if elements := elementsOf(current); len(elements) > 0 {
		first = elements[0]
	}
	return []any{o.marked(first, marker)}
}

func (o readOnly) marked(object map[string]any, marker string) map[string]any {
	marked := maps.Clone(object)
	if marked == nil {
		marked = map[string]any{}
	}
	marked[o.sub.Name] = marker
	return marked
}

func readOnlyWithin(extension core.SchemaURI, attribute *core.Attribute) []readOnly {
	if _, common := core.CommonAttribute(attribute.Name); common {
		return nil
	}
	if isReadOnly(attribute) {
		return []readOnly{{extension: extension, attribute: attribute}}
	}
	if attribute.Type != core.TypeComplex || attribute.Mutability != core.MutabilityReadWrite {
		return nil
	}
	found := []readOnly{}
	for _, sub := range attribute.SubAttributes {
		if isReadOnly(sub) {
			found = append(found, readOnly{extension: extension, attribute: attribute, sub: sub})
		}
	}
	return found
}
