package scimtest

import (
	"net/http"
	"slices"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type filtering struct {
	*resource
}

func (r filtering) run(t *testing.T) {
	t.Helper()

	t.Run("finds a resource by id", r.checkID)
	t.Run("rejects an invalid filter", r.checkInvalid)
	t.Run("rejects gt on a boolean", r.checkBooleanGreater)
	t.Run("operators", r.checkOperators)
	t.Run("matches a value path within one value", r.checkValuePath)
	t.Run("compares a boolean", r.checkBooleanEquals)
	t.Run("compares meta attributes", r.checkMeta)
	t.Run("binds and tighter than or", r.checkPrecedence)
	t.Run("compares a canonical type", r.checkCanonicalTypes)
}

func (r filtering) checkID(t *testing.T) {
	if r.suite.unsupported(t, "filter", r.config.Filter.Supported) {
		res := r.send(t, byID("scimtest-none"))
		expectStatus(t, res, http.StatusNotImplemented, "RFC 7644 3.12")
		return
	}
	_, created := r.create(t, newUnique())
	query := byID(idOf(created))

	if got := r.filtered(t, query); !slices.Equal(got, []string{idOf(created)}) {
		t.Errorf("scimtest: %s (RFC 7644 3.4.2.2): ids %v, want [%s]", r.describe(query), got, idOf(created))
	}
}

func (r filtering) checkInvalid(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	r.suite.should(t, "RFC 7644 Table 9 400 invalidFilter for an invalid filter")
	RequireError(t, r.send(t, filterBy("id eq")), http.StatusBadRequest, scimerrors.InvalidFilter)
}

func (r filtering) checkBooleanGreater(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	attribute := r.attribute(t, "singular boolean attribute", func(a *core.Attribute) bool { return a.Type == core.TypeBoolean && !a.MultiValued })
	RequireError(t, r.send(t, filterBy(attribute.Name+" gt true")), http.StatusBadRequest, scimerrors.InvalidFilter)
}

func (r filtering) checkOperators(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	newFilterTable(t, r.resource).run(t)
}

func (r filtering) checkValuePath(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	attribute := r.attribute(t, "multi-valued attribute with a canonical type and a value", func(a *core.Attribute) bool {
		typed, value := a.SubAttributes.Lookup(typeSub), a.SubAttributes.Lookup(valueSub)
		return a.MultiValued && a.Mutability == core.MutabilityReadWrite && typed != nil && len(typed.CanonicalValues) > 1 && value != nil && isFilterable(value)
	})
	kinds := attribute.SubAttribute(typeSub).CanonicalValues[:2]
	unique := newUnique()
	alpha, beta := "scimtest alpha "+unique, "scimtest beta "+unique
	ids := make([]string, 0, 2)
	for _, values := range [][]string{{alpha, beta}, {beta, alpha}} {
		body := r.body(t, newUnique())
		body.Set(attribute.Name, []any{map[string]any{typeSub: kinds[0], valueSub: values[0]}, map[string]any{typeSub: kinds[1], valueSub: values[1]}})
		ids = append(ids, idOf(r.seed(t, body)))
	}
	filter := attribute.Name + "[type eq " + quoted(kinds[0]) + " and value eq " + quoted(alpha) + "]"

	r.expectMatches(t, filter, ids, ids[:1])
}

func (r filtering) checkBooleanEquals(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	attribute := r.attribute(t, "readWrite singular boolean attribute", func(a *core.Attribute) bool {
		return a.Type == core.TypeBoolean && !a.MultiValued && a.Mutability == core.MutabilityReadWrite
	})
	ids := make([]string, 0, 2)
	for _, value := range []bool{true, false} {
		body := r.body(t, newUnique())
		body.Set(attribute.Name, value)
		ids = append(ids, idOf(r.seed(t, body)))
	}

	r.expectMatches(t, attribute.Name+" eq true", ids, ids[:1])
	r.expectMatches(t, attribute.Name+" eq false", ids, ids[1:])
	r.expectMatches(t, "not ("+attribute.Name+" eq true)", ids, ids[1:])
}

func (r filtering) checkMeta(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	_, first := r.create(t, newUnique())
	_, second := r.create(t, newUnique())
	ids := []string{idOf(first), idOf(second)}
	created, _ := metaOf(first)["created"].(string)
	modified, _ := metaOf(first)["lastModified"].(string)

	r.expectMatches(t, "meta.resourceType eq "+quoted(string(r.kind.Name)), ids, ids)
	r.expectMatches(t, "meta.resourceType eq \"scimtest\"", ids, nil)
	if metaOf(second)["created"] == created || metaOf(second)["lastModified"] == modified {
		t.Skipf("n/a: both resources were created at %s", created)
	}
	r.expectMatches(t, "meta.lastModified gt "+quoted(modified), ids, ids[1:])
	r.expectMatches(t, "meta.created le "+quoted(created), ids, ids[:1])
}

// RFC 7644 Section 3.4.2.2: "not" takes precedence over "and", which takes precedence over "or".
func (r filtering) checkPrecedence(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	ids := r.createIDs(t, 3)

	r.expectMatches(t, idFilter(ids[0])+" or "+idFilter(ids[1])+" and "+idFilter(ids[2]), ids, ids[:1])
	r.expectMatches(t, "not ("+idFilter(ids[0])+") and "+idFilter(ids[1]), ids, ids[1:2])
}

func (r filtering) checkCanonicalTypes(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	typed := slices.DeleteFunc(slices.Clone(r.schemas.Base().Attributes), func(a *core.Attribute) bool {
		kind := a.SubAttributes.Lookup(typeSub)
		return !a.MultiValued || a.Mutability != core.MutabilityReadWrite || kind == nil || len(kind.CanonicalValues) < 2
	})
	if len(typed) == 0 {
		t.Skipf("n/a: %s has no multi-valued attribute with a canonical type", r.kind.Name)
	}
	for _, attribute := range typed {
		t.Run(attribute.Name+".type", func(t *testing.T) { r.checkCanonicalType(t, attribute) })
	}
}

func (r filtering) checkCanonicalType(t *testing.T, attribute *core.Attribute) {
	t.Helper()

	kinds := attribute.SubAttribute(typeSub).CanonicalValues[:2]
	ids := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		body := r.body(t, newUnique())
		body.Set(attribute.Name, []any{map[string]any{typeSub: kind}})
		ids = append(ids, idOf(r.seed(t, body)))
	}
	path := attribute.Name + "." + typeSub

	r.expectMatches(t, path+" eq "+quoted(kinds[0]), ids, ids[:1])
	r.expectMatches(t, path+" ne "+quoted(kinds[0]), ids, ids[1:])
	r.expectMatches(t, path+" pr", ids, ids)
}
