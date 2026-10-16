package scimtest

import (
	"net/url"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
)

type singular struct {
	link
	extension *core.Schema
	attribute *core.Attribute
}

func newSingulars(resources []*resource) []singular {
	found := []singular{}
	for _, source := range resources {
		for _, schema := range source.schemas {
			found = append(found, singularsIn(resources, source, schema)...)
		}
	}
	return found
}

func singularsIn(resources []*resource, source *resource, schema *core.Schema) []singular {
	found := []singular{}
	for _, attribute := range schema.Attributes {
		if attribute.MultiValued || attribute.Mutability != core.MutabilityReadWrite {
			continue
		}
		if targets := targetsOf(resources, attribute.ReferenceTargets()); len(targets) > 0 {
			found = append(found, singular{source: source, target: targets[0], extension: extensionOf(source.schemas, schema), attribute: attribute})
		}
	}
	return found
}

func (r singular) run(t *testing.T) {
	t.Helper()

	t.Run("stores the referenced value", r.checkStored)
	t.Run("finds the source by the value", r.checkFiltered)
	t.Run("replaces the value", r.checkReplace)
	t.Run("removes the value and keeps the other attributes", r.checkRemove)
	t.Run("returns only the requested value", r.checkProjected)
}

func (r singular) checkStored(t *testing.T) {
	id := r.newTarget(t)

	r.expectValue(t, r.newSource(t, id), id)
}

func (r singular) checkFiltered(t *testing.T) {
	supports(t, "filter", r.source.config.Filter.Supported)
	id := r.newTarget(t)
	ids := []string{idOf(r.newSource(t, id)), idOf(r.newSource(t))}

	r.source.expectMatches(t, r.path()+".value eq "+quoted(id), ids, ids[:1])
	r.source.expectMatches(t, r.path()+" pr", ids, ids[:1])
}

// RFC 7644 Section 3.5.2.3: replace sets the value of the targeted sub-attribute.
func (r singular) checkReplace(t *testing.T) {
	supports(t, "patch", r.source.config.Patch.Supported)
	source := r.newSource(t, r.newTarget(t))
	second := r.newTarget(t)

	r.patch(t, source, op(patch.OpReplace, r.path()+"."+valueSub, second))
	r.expectValue(t, source, second)
}

// RFC 7644 Section 3.5.2.2: remove unassigns the targeted attribute and leaves the others.
func (r singular) checkRemove(t *testing.T) {
	supports(t, "patch", r.source.config.Patch.Supported)
	for _, path := range []string{r.path(), r.path() + "." + valueSub} {
		t.Run(path, func(t *testing.T) {
			sibling := r.sibling()
			source := r.newSource(t, r.newTarget(t))

			r.patch(t, source, op(patch.OpRemove, path, nil))
			read := r.source.read(t, source)
			if got := r.holder(read).Get(r.attribute.Name); got != nil {
				t.Errorf("scimtest: PATCH %s (RFC 7644 3.5.2.2): %s is %v, want unassigned", r.source.path(source), r.path(), got)
			}
			if sibling != nil && r.holder(read).Get(sibling.Name) == nil {
				t.Errorf("scimtest: PATCH %s (RFC 7644 3.5.2.2): removing %s also removed %s", r.source.path(source), path, sibling.Name)
			}
		})
	}
}

// RFC 7644 Section 3.10: a sub-attribute of an extension attribute is named by its schema URI and path.
func (r singular) checkProjected(t *testing.T) {
	id := r.newTarget(t)
	source := r.newSource(t, id)
	path := r.source.path(source) + "?" + url.Values{attributesParam: {r.path() + "." + valueSub}}.Encode()

	got := core.Object(objectOf(r.holder(get[core.Object](t, r.source.client(), path, "RFC 7644 3.4.2.5")).Get(r.attribute.Name)))
	if len(got) != 1 || got.Get(valueSub) != id {
		t.Errorf("scimtest: GET %s (RFC 7644 3.4.2.5): %s is %v, want only value %s", path, r.path(), map[string]any(got), id)
	}
}

func (r singular) newSource(t *testing.T, ids ...string) core.Object {
	t.Helper()

	body := r.source.body(t, newUnique())
	holder := map[string]any{}
	if sibling := r.sibling(); sibling != nil {
		holder[sibling.Name] = placeholder + " " + newUnique()
	}
	for _, id := range ids {
		holder[r.attribute.Name] = map[string]any{valueSub: id}
	}
	if r.extension == nil {
		for name, value := range holder {
			body.Set(name, value)
		}
	} else {
		body.Set(string(r.extension.ID), holder)
	}
	body.Set(schemasAttr, r.source.schemaURIs(body))
	_, created := r.source.createBody(t, body)
	return created
}

func (r singular) expectValue(t *testing.T, source core.Object, id string) {
	t.Helper()

	got := core.Object(objectOf(r.holder(r.source.read(t, source)).Get(r.attribute.Name)))
	if got.Get(valueSub) != id {
		t.Errorf("scimtest: GET %s (RFC 7643 2.3.7): %s is %v, want value %s", r.source.path(source), r.path(), map[string]any(got), id)
	}
}

func (r singular) path() string {
	if r.extension == nil {
		return r.attribute.Name
	}
	return string(r.extension.ID) + ":" + r.attribute.Name
}

func (r singular) holder(object core.Object) core.Object {
	if r.extension == nil {
		return object
	}
	return core.Object(objectOf(object.Get(string(r.extension.ID))))
}

func (r singular) sibling() *core.Attribute {
	if r.extension == nil {
		return nil
	}
	return firstOf(r.extension.Attributes, isFilterable)
}

func extensionOf(schemas core.Schemas, schema *core.Schema) *core.Schema {
	if schemas.IsExtension(schema) {
		return schema
	}
	return nil
}
