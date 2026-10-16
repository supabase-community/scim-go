package scimtest

import (
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

var comparisons = map[string]func(value, operand string) bool{
	"eq": func(value, operand string) bool { return value == operand },
	"ne": func(value, operand string) bool { return value != operand },
	"co": strings.Contains,
	"sw": strings.HasPrefix,
	"ew": strings.HasSuffix,
	"gt": func(value, operand string) bool { return value > operand },
	"ge": func(value, operand string) bool { return value >= operand },
	"lt": func(value, operand string) bool { return value < operand },
	"le": func(value, operand string) bool { return value <= operand },
}

type filtering struct {
	*resource
}

func (r filtering) run(t *testing.T) {
	t.Helper()

	t.Run("finds a resource by id", r.checkID)
	t.Run("rejects an invalid filter", r.checkInvalid)
	t.Run("rejects ordering a boolean or binary", r.checkOrderingRejected)
	t.Run("rejects an unknown operator", r.checkUnknownOperator)
	t.Run("operators", r.checkOperators)
	t.Run("matches a value path within one value", r.checkValuePath)
	t.Run("compares a boolean", r.checkBooleanEquals)
	t.Run("compares meta attributes", r.checkMeta)
	t.Run("binds and tighter than or", r.checkPrecedence)
	t.Run("compares a canonical type", r.checkCanonicalTypes)
	t.Run("treats an unset primary as false", r.checkPrimaryDefault)
	t.Run("matches pr only when a value is present", r.checkPresent)
	t.Run("treats attribute names and operators as case-insensitive", r.checkNameCase)
	t.Run("forbids a GET filter on a returned:never attribute", r.checkSensitive)
	t.Run("finds every stored value by eq and pr", r.checkStored)
	t.Run("compares an unassigned value with null", r.checkNull)
	t.Run("folds non-ASCII case", r.checkUnicodeCase)
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
	for _, filter := range []string{`id eq`, `id co`, `(id pr`, `id pr)`, `id pr and`, `not id pr`, `id[value[id pr]]`, `id.$ref pr`} {
		t.Run(filter, func(t *testing.T) {
			RequireError(t, r.send(t, filterBy(filter)), http.StatusBadRequest, scimerrors.InvalidFilter)
		})
	}
}

// RFC 7644 Section 3.4.2.2: gt, ge, lt and le on a boolean or binary SHALL fail with invalidFilter.
func (r filtering) checkOrderingRejected(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	paths := pathsOf(r.schemas.Base(), func(a *core.Attribute) bool {
		return a.Type == core.TypeBoolean || a.Type == core.TypeBinary
	})
	if len(paths) == 0 {
		t.Skipf("n/a: %s has no boolean or binary attribute", r.kind.Name)
	}
	for _, path := range slices.Sorted(maps.Keys(paths)) {
		literal := `"c2NpbXRlc3Q="`
		if paths[path].Type == core.TypeBoolean {
			literal = "true"
		}
		for _, op := range []string{"gt", "ge", "lt", "le"} {
			t.Run(path+" "+op, func(t *testing.T) {
				RequireError(t, r.send(t, filterBy(path+" "+op+" "+literal)), http.StatusBadRequest, scimerrors.InvalidFilter)
			})
		}
	}
}

// RFC 7644 Section 3.4.2.2: providers MUST decline to filter with an unrecognized operator.
func (r filtering) checkUnknownOperator(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	RequireError(t, r.send(t, filterBy(`id regex "scimtest"`)), http.StatusBadRequest, scimerrors.InvalidFilter)
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
	later, _ := metaOf(second)["created"].(string)
	r.expectMatches(t, "meta.lastModified gt "+quoted(modified), ids, ids[1:])
	r.expectMatches(t, "meta.created le "+quoted(created), ids, ids[:1])
	r.expectMatches(t, "meta.created eq "+quoted(created), ids, ids[:1])
	r.expectMatches(t, "meta.created ne "+quoted(created), ids, ids[1:])
	r.expectMatches(t, "meta.created ge "+quoted(created), ids, ids)
	r.expectMatches(t, "meta.created lt "+quoted(later), ids, ids[:1])
}

// RFC 7644 Section 3.4.2.2: "not" takes precedence over "and", which takes precedence over "or".
func (r filtering) checkPrecedence(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	ids := r.createIDs(t, 3)

	r.expectMatches(t, idFilter(ids[0])+" or "+idFilter(ids[1])+" and "+idFilter(ids[2]), ids, ids[:1])
	r.expectMatches(t, "not ("+idFilter(ids[0])+") and "+idFilter(ids[1]), ids, ids[1:2])
	r.expectMatches(t, "("+idFilter(ids[0])+" or "+idFilter(ids[1])+") and "+idFilter(ids[1]), ids, ids[1:2])
}

func (r filtering) checkCanonicalTypes(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	typed := slices.DeleteFunc(slices.Clone(r.schemas.Base().Attributes), func(a *core.Attribute) bool {
		kind := a.SubAttributes.Lookup(typeSub)
		return !a.MultiValued || a.Mutability != core.MutabilityReadWrite || kind == nil || len(kind.CanonicalValues) < 2 || isReference(a)
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

	sub := attribute.SubAttribute(typeSub)
	kinds := sub.CanonicalValues[:2]
	ids := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		body := r.body(t, newUnique())
		body.Set(attribute.Name, []any{map[string]any{typeSub: kind}})
		ids = append(ids, idOf(r.seed(t, body)))
	}
	path := attribute.Name + "." + typeSub
	operand := kinds[0]
	suffix, prefix := operand, operand
	if len(operand) > 1 {
		suffix, prefix = operand[1:], operand[:len(operand)-1]
	}

	for _, op := range []struct{ name, operand string }{
		{"eq", operand},
		{"ne", operand},
		{"co", suffix},
		{"sw", prefix},
		{"ew", suffix},
		{"gt", operand},
		{"ge", operand},
		{"lt", operand},
		{"le", operand},
	} {
		want := []string{}
		for i, kind := range kinds {
			if compares(op.name, kind, op.operand, sub.CaseExact) {
				want = append(want, ids[i])
			}
		}
		r.expectMatches(t, path+" "+op.name+" "+quoted(op.operand), ids, want)
	}
	r.expectMatches(t, path+" pr", ids, ids)
}

// RFC 7643 Section 2.4: an unspecified "primary" SHALL be assumed to be false.
func (r filtering) checkPrimaryDefault(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	attribute := r.attribute(t, "readWrite multi-valued attribute with a value and a primary", hasPrimary)
	unique := newUnique()
	ids := make([]string, 0, 2)
	for _, value := range []map[string]any{{primarySub: true}, {}} {
		value[valueSub] = email("scimtest-"+newUnique(), unique)
		body := r.body(t, newUnique())
		body.Set(attribute.Name, []any{value})
		ids = append(ids, idOf(r.seed(t, body)))
	}

	r.expectMatches(t, attribute.Name+"."+primarySub+" eq false", ids, ids[1:])
	r.expectMatches(t, attribute.Name+"[primary eq false]", ids, ids[1:])
}

// RFC 7644 Section 3.4.2.2: pr matches a non-empty value, or a non-empty node for complex attributes.
func (r filtering) checkPresent(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	tested := false
	for _, attribute := range r.schemas.Base().Attributes {
		value := presentValue(attribute)
		if attribute.Required || value == nil {
			continue
		}
		tested = true
		t.Run(attribute.Name, func(t *testing.T) {
			assigned := r.body(t, newUnique())
			assigned.Set(attribute.Name, value)
			unassigned := r.body(t, newUnique())
			unassigned.Remove(attribute.Name)
			ids := []string{idOf(r.seed(t, assigned)), idOf(r.seed(t, unassigned))}

			r.expectMatches(t, attribute.Name+" pr", ids, ids[:1])
		})
	}
	if !tested {
		t.Skipf("n/a: %s has no optional readWrite string or complex attribute", r.kind.Name)
	}
}

func (r filtering) checkNameCase(t *testing.T) {
	r.suite.should(t, "RFC 7644 3.4.2.2 attribute names and operators are case insensitive")
	supports(t, "filter", r.config.Filter.Supported)
	ids := r.createIDs(t, 2)

	r.expectMatches(t, `ID EQ `+quoted(ids[0]), ids, ids[:1])
	r.expectMatches(t, `Id Eq `+quoted(ids[0])+` oR iD eQ `+quoted(ids[1]), ids, ids)
	r.checkBaseURN(t)
	extension, attribute := extensionAttribute(r.schemas, isFilterable)
	if extension == nil {
		return
	}
	body := r.body(t, newUnique())
	value := "scimtest " + newUnique()
	r.withExtension(body, extension, attribute.Name, value)
	id := idOf(r.seed(t, body))
	path := strings.ToUpper(string(extension.ID)) + ":" + strings.ToUpper(attribute.Name)

	r.expectMatches(t, path+" eq "+quoted(value), []string{id}, []string{id})
}

// RFC 7644 Section 3.10: clients MAY prefix a core attribute with its schema URN, and every facet is case insensitive.
func (r filtering) checkBaseURN(t *testing.T) {
	t.Helper()

	attribute := firstOf(r.schemas.Base().Attributes, patchable)
	if attribute == nil {
		return
	}
	body := r.body(t, newUnique())
	value := "scimtest " + newUnique()
	body.Set(attribute.Name, value)
	id := idOf(r.seed(t, body))
	urn := string(r.kind.Schema) + ":" + attribute.Name

	r.expectMatches(t, urn+" eq "+quoted(value), []string{id}, []string{id})
	r.expectMatches(t, strings.ToUpper(urn)+" eq "+quoted(value), []string{id}, []string{id})
}

// RFC 7644 Section 7.5.2: a GET filter with sensitive information SHOULD be answered with 403.
func (r filtering) checkSensitive(t *testing.T) {
	r.suite.should(t, "RFC 7644 7.5.2 403 for a GET filter on a returned:never attribute")
	supports(t, "filter", r.config.Filter.Supported)
	if r.transport.search {
		t.Skip("n/a: POST .search keeps the filter out of the URI")
	}
	attribute := r.attribute(t, "returned:never attribute", func(a *core.Attribute) bool { return a.Returned == core.ReturnedNever })

	RequireError(t, r.send(t, filterBy(attribute.Name+` eq "scimtest"`)), http.StatusForbidden, "")
}

// RFC 7644 Section 3.4.2.2: eq matches an identical value and pr matches any non-empty value.
func (r filtering) checkStored(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	_, full := r.createBody(t, r.fullBody(t))
	_, base := r.create(t, newUnique())
	ids := []string{idOf(full), idOf(base)}
	others := keysOf(storedLeaves(r.schemas, r.read(t, base)))
	paths := map[string]bool{}

	for _, l := range storedLeaves(r.schemas, r.read(t, full)) {
		r.expectMatches(t, l.path+" eq "+l.literal(), ids, matching(ids, others[l.key()]))
		if !paths[l.path] {
			paths[l.path] = true
			r.expectMatches(t, l.path+" pr", ids, matching(ids, hasPath(others, l.path)))
		}
	}
}

// RFC 7643 Section 2.5: unassigned and null are equivalent in state.
func (r filtering) checkNull(t *testing.T) {
	r.suite.should(t, "RFC 7643 2.5 eq null matches an unassigned attribute")
	supports(t, "filter", r.config.Filter.Supported)
	attribute := r.attribute(t, "optional readWrite singular string attribute", func(a *core.Attribute) bool { return patchable(a) && !a.Required })
	assigned := r.body(t, newUnique())
	assigned.Set(attribute.Name, "scimtest "+newUnique())
	unassigned := r.body(t, newUnique())
	unassigned.Remove(attribute.Name)
	ids := []string{idOf(r.seed(t, assigned)), idOf(r.seed(t, unassigned))}

	r.expectMatches(t, attribute.Name+" eq null", ids, ids[1:])
	r.expectMatches(t, attribute.Name+" ne null", ids, ids[:1])
}

// RFC 7644 Section 3.4.2.2: caseExact false compares without case, including non-ASCII letters.
func (r filtering) checkUnicodeCase(t *testing.T) {
	r.suite.should(t, "RFC 7644 3.4.2.2 caseExact false folds non-ASCII case")
	supports(t, "filter", r.config.Filter.Supported)
	attribute := r.attribute(t, "readWrite singular case-insensitive string attribute", func(a *core.Attribute) bool { return patchable(a) && !a.CaseExact })
	unique := newUnique()
	body := r.body(t, newUnique())
	body.Set(attribute.Name, "scimtest \u00c9mile "+unique)
	id := idOf(r.seed(t, body))

	for _, op := range []string{"eq", "sw", "co"} {
		r.expectMatches(t, attribute.Name+" "+op+" "+quoted("SCIMTEST \u00e9MILE "+strings.ToUpper(unique)), []string{id}, []string{id})
	}
}

func compares(op, value, operand string, caseExact bool) bool {
	if !caseExact {
		value, operand = strings.ToLower(value), strings.ToLower(operand)
	}
	return comparisons[op](value, operand)
}

func presentValue(attribute *core.Attribute) any {
	value := "scimtest " + newUnique()
	if isFilterable(attribute) {
		return value
	}
	if attribute.Type != core.TypeComplex || attribute.Mutability != core.MutabilityReadWrite {
		return nil
	}
	sub := firstOf(attribute.SubAttributes, isFilterable)
	if sub == nil {
		return nil
	}
	if attribute.MultiValued {
		return []any{map[string]any{sub.Name: value}}
	}
	return map[string]any{sub.Name: value}
}

func matching(ids []string, both bool) []string {
	if both {
		return ids
	}
	return ids[:1]
}
