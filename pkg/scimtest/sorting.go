package scimtest

import (
	"net/http"
	"net/url"
	"slices"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
)

type sorting struct {
	*resource
}

func (r sorting) run(t *testing.T) {
	t.Helper()

	t.Run("accepts sortBy", r.checkSortBy)
	t.Run("sorts a case-insensitive attribute without case", r.checkCase)
	t.Run("sorts by a sub-attribute", r.checkSubAttribute)
	t.Run("sorts a multi-valued attribute by its primary value", r.checkPrimary)
	t.Run("orders resources without a value last", r.checkUnassigned)
}

func (r sorting) checkSortBy(t *testing.T) {
	query := url.Values{"sortBy": {"id"}}
	if r.suite.unsupported(t, "sort", r.config.Sort.Supported) {
		expectStatus(t, r.send(t, query), http.StatusNotImplemented, "RFC 7644 3.12")
		return
	}
	r.list(t, query)
}

// RFC 7644 Section 3.4.2.3: case-insensitive attributes sort with case-insensitive Unicode alphabetic order.
func (r sorting) checkCase(t *testing.T) {
	supports(t, "sort", r.config.Sort.Supported)
	supports(t, "filter", r.config.Filter.Supported)
	attribute := r.attribute(t, "readWrite singular case-insensitive string attribute", func(a *core.Attribute) bool { return patchable(a) && !a.CaseExact })
	unique := newUnique()
	upper := r.withValue(t, attribute.Name, "scimtest B "+unique)
	lower := r.withValue(t, attribute.Name, "scimtest a "+unique)

	r.expectOrder(t, attribute.Name, []string{lower, upper})
}

func (r sorting) checkSubAttribute(t *testing.T) {
	supports(t, "sort", r.config.Sort.Supported)
	supports(t, "filter", r.config.Filter.Supported)
	attribute := r.attribute(t, "readWrite singular complex attribute with a string sub-attribute", func(a *core.Attribute) bool {
		return a.Type == core.TypeComplex && !a.MultiValued && a.Mutability == core.MutabilityReadWrite && firstOf(a.SubAttributes, isFilterable) != nil
	})
	sub := firstOf(attribute.SubAttributes, isFilterable)
	unique := newUnique()
	ids := []string{
		r.withValue(t, attribute.Name, map[string]any{sub.Name: "scimtest b " + unique}),
		r.withValue(t, attribute.Name, map[string]any{sub.Name: "scimtest a " + unique}),
	}

	r.expectOrder(t, attribute.Name+"."+sub.Name, []string{ids[1], ids[0]})
}

func (r sorting) checkPrimary(t *testing.T) {
	r.suite.should(t, "RFC 7644 3.4.2.3 a multi-valued sortBy uses the primary value")
	supports(t, "sort", r.config.Sort.Supported)
	supports(t, "filter", r.config.Filter.Supported)
	attribute := r.attribute(t, "readWrite multi-valued attribute with a value and a primary", hasPrimary)
	unique := newUnique()
	ids := []string{
		r.withValue(t, attribute.Name, []any{map[string]any{valueSub: email("scimtest-a", unique)}, map[string]any{valueSub: email("scimtest-c", unique), primarySub: true}}),
		r.withValue(t, attribute.Name, []any{map[string]any{valueSub: email("scimtest-b", unique)}}),
	}

	r.expectOrder(t, attribute.Name+"."+valueSub, []string{ids[1], ids[0]})
}

func (r sorting) checkUnassigned(t *testing.T) {
	r.suite.should(t, "RFC 7644 3.4.2.3 resources without a sortBy value are last when ascending")
	attribute := r.sortable(t)
	if attribute.Required {
		t.Skipf("n/a: %s is required", attribute.Name)
	}
	body := r.body(t, newUnique())
	body.Remove(attribute.Name)
	_, unassigned := r.createBody(t, body)
	assigned := r.withValue(t, attribute.Name, "scimtest "+newUnique())

	r.expectOrder(t, attribute.Name, []string{assigned, idOf(unassigned)})
}

func (r sorting) sortable(t *testing.T) *core.Attribute {
	t.Helper()

	supports(t, "sort", r.config.Sort.Supported)
	supports(t, "filter", r.config.Filter.Supported)
	return r.attribute(t, "readWrite singular string attribute", patchable)
}

func (r sorting) withValue(t *testing.T, name string, value any) string {
	t.Helper()

	body := r.body(t, newUnique())
	body.Set(name, value)
	_, created := r.createBody(t, body)
	return idOf(created)
}

func (r sorting) expectOrder(t *testing.T, sortBy string, ascending []string) {
	t.Helper()

	descending := slices.Clone(ascending)
	slices.Reverse(descending)
	query := byID(ascending...)
	query.Set("sortBy", sortBy)
	for _, order := range []struct {
		sortOrder string
		want      []string
	}{{"", ascending}, {"ascending", ascending}, {"descending", descending}} {
		if order.sortOrder != "" {
			query.Set("sortOrder", order.sortOrder)
		}
		if got := idsOf(r.list(t, query).Resources); !slices.Equal(got, order.want) {
			t.Errorf("scimtest: %s (RFC 7644 3.4.2.3): ids %v, want %v", r.describe(query), got, order.want)
		}
	}
}
