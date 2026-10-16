package scimtest

import (
	"net/url"
	"slices"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
)

type projecting struct {
	*resource
}

type projection struct {
	present, absent []string
}

func (r projecting) run(t *testing.T) {
	t.Helper()

	body := r.populated(t)
	attribute := r.attribute(t, "returned:default attribute in its fixture", func(a *core.Attribute) bool {
		return a.Returned == core.ReturnedDefault && body.Has(a.Name)
	})
	_, created := r.createBody(t, body)

	t.Run("returns a requested attribute and id", func(t *testing.T) {
		r.expectAttributes(t, r.path(created)+"?attributes="+attribute.Name, []string{"id", attribute.Name}, r.others(created, attribute.Name))
	})
	t.Run("removes only the excluded attribute", func(t *testing.T) {
		for _, name := range r.others(created, "") {
			r.expectAttributes(t, r.path(created)+"?"+url.Values{excludedParam: {name}}.Encode(), append([]string{"id"}, r.others(created, name)...), []string{name})
		}
	})
	t.Run("never excludes id", func(t *testing.T) {
		r.expectAttributes(t, r.path(created)+"?excludedAttributes=id", []string{"id"}, nil)
	})
	t.Run("returns a requested sub-attribute only", r.checkSubAttribute)
	t.Run("projects an extension attribute", r.checkExtension)
	for _, tr := range transports {
		t.Run("never excludes id from a query over "+tr.name, func(t *testing.T) { r.checkListedID(t, tr, idOf(created)) })
		t.Run("projects a query over "+tr.name, func(t *testing.T) { r.checkListed(t, tr, created, attribute.Name) })
	}
}

// RFC 7643 Section 3.1: "id" is returned "always", so a query cannot exclude it.
func (r projecting) checkListedID(t *testing.T, tr transport, id string) {
	t.Helper()

	supports(t, "filter", r.config.Filter.Supported)
	query := byID(id)
	query.Set(excludedParam, "id")

	over := r.over(t, tr)
	if got := idsOf(over.list(t, query).Resources); !slices.Equal(got, []string{id}) {
		t.Errorf("scimtest: %s (RFC 7644 3.4.2.5): ids %v, want [%s]", over.describe(query), got, id)
	}
}

func (r projecting) checkListed(t *testing.T, tr transport, created core.Object, name string) {
	t.Helper()

	supports(t, "filter", r.config.Filter.Supported)
	over := r.over(t, tr)
	for param, want := range map[string]projection{
		attributesParam: {[]string{"id", name}, r.others(created, name)},
		excludedParam:   {[]string{"id"}, []string{name}},
	} {
		query := byID(idOf(created))
		query.Set(param, name)
		page := over.list(t, query)
		if len(page.Resources) != 1 {
			t.Fatalf("scimtest: %s (RFC 7644 3.4.2.5): %d resources, want 1", over.describe(query), len(page.Resources))
		}
		want.expect(t, over.describe(query), page.Resources[0])
	}
}

// RFC 7644 Section 3.4.2.5: attributes and sub-attributes are named in standard attribute notation.
func (r projecting) checkSubAttribute(t *testing.T) {
	attribute, created, kept, dropped := r.complexTarget(t)

	for param, named := range map[string]string{attributesParam: kept, excludedParam: dropped} {
		path := r.path(created) + "?" + url.Values{param: {attribute.Name + "." + named}}.Encode()
		parent := core.Object(objectOf(r.get(t, path).Get(attribute.Name)))
		if !parent.Has(kept) || parent.Has(dropped) {
			t.Errorf("scimtest: GET %s (RFC 7644 3.4.2.5): %s is %v, want %s without %s", path, attribute.Name, map[string]any(parent), kept, dropped)
		}
	}
}

// RFC 7644 Section 3.10: an extension attribute is named by its schema URI and attribute name.
func (r projecting) checkExtension(t *testing.T) {
	extension, attribute := r.extended(t, isFilterable)
	uri := string(extension.ID)
	body := r.body(t, newUnique())
	r.withExtension(body, extension, attribute.Name, placeholder+" "+newUnique())
	_, created := r.createBody(t, body)
	path := uri + ":" + attribute.Name

	requested := r.path(created) + "?" + url.Values{attributesParam: {path}}.Encode()
	got := r.get(t, requested)
	if !core.Object(objectOf(got.Get(uri))).Has(attribute.Name) {
		t.Errorf("scimtest: GET %s (RFC 7644 3.4.2.5): %s is missing", requested, path)
	}
	projection{[]string{"id"}, r.others(created, uri)}.expect(t, "GET "+requested, got)
	excluded := r.path(created) + "?" + url.Values{excludedParam: {path}}.Encode()
	if core.Object(objectOf(r.get(t, excluded).Get(uri))).Has(attribute.Name) {
		t.Errorf("scimtest: GET %s (RFC 7644 3.4.2.5): %s is present", excluded, path)
	}
}

func (r projecting) populated(t *testing.T) core.Object {
	t.Helper()

	body := r.body(t, newUnique())
	for _, attribute := range r.schemas.Base().Attributes {
		if body.Has(attribute.Name) || attribute.MultiValued || attribute.Mutability != core.MutabilityReadWrite || attribute.Returned != core.ReturnedDefault {
			continue
		}
		if subs := filterableSubs(attribute); len(subs) > 0 {
			body.Set(attribute.Name, filled(subs))
		}
	}
	if extension, attribute := extensionAttribute(r.schemas, isFilterable); extension != nil {
		r.withExtension(body, extension, attribute.Name, placeholder+" "+newUnique())
	}
	return body
}

func (r projecting) others(created core.Object, except string) []string {
	others := []string{}
	for _, attribute := range r.schemas.Base().Attributes {
		if attribute.Returned != core.ReturnedAlways && attribute.Name != except && created.Has(attribute.Name) {
			others = append(others, attribute.Name)
		}
	}
	for _, extension := range r.schemas.Extensions() {
		if string(extension.ID) != except && created.Has(string(extension.ID)) {
			others = append(others, string(extension.ID))
		}
	}
	return others
}

func (r projecting) expectAttributes(t *testing.T, path string, present, absent []string) {
	t.Helper()
	projection{present, absent}.expect(t, "GET "+path, r.get(t, path))
}

func (r projecting) get(t *testing.T, path string) core.Object {
	t.Helper()
	return get[core.Object](t, r.client(), path, "RFC 7644 3.4.2.5")
}

func (p projection) expect(t *testing.T, where string, got core.Object) {
	t.Helper()

	for _, name := range p.present {
		if !got.Has(name) {
			t.Errorf("scimtest: %s (RFC 7644 3.4.2.5): %s is missing", where, name)
		}
	}
	for _, name := range p.absent {
		if got.Has(name) {
			t.Errorf("scimtest: %s (RFC 7644 3.4.2.5): %s is present", where, name)
		}
	}
}
