package scimtest

import (
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
)

type filterCase struct {
	op   string
	expr func(target *filterTarget) string
	want []int
	fold bool
}

type filterTarget struct {
	path      string
	caseExact bool
	unique    string
	alpha     string
	beta      string
	ids       []string
	rejected  bool
}

type filterTable struct {
	r       *resource
	targets []*filterTarget
}

var filterCases = []filterCase{
	{op: "eq", expr: func(f *filterTarget) string { return f.compare("eq", f.alpha) }, want: []int{0}},
	{op: "eq case", expr: func(f *filterTarget) string { return f.compare("eq", strings.ToUpper(f.alpha)) }, want: []int{0}, fold: true},
	{op: "ne", expr: func(f *filterTarget) string { return f.compare("ne", f.alpha) }, want: []int{1}},
	{op: "co", expr: func(f *filterTarget) string { return f.compare("co", "alpha "+f.unique) }, want: []int{0}},
	{op: "sw", expr: func(f *filterTarget) string { return f.compare("sw", "scimtest beta") }, want: []int{1}},
	{op: "ew", expr: func(f *filterTarget) string { return f.compare("ew", " "+f.unique) }, want: []int{0, 1}},
	{op: "pr", expr: func(f *filterTarget) string { return f.path + " pr" }, want: []int{0, 1}},
	{op: "gt", expr: func(f *filterTarget) string { return f.compare("gt", f.alpha) }, want: []int{1}},
	{op: "ge", expr: func(f *filterTarget) string { return f.compare("ge", f.alpha) }, want: []int{0, 1}},
	{op: "lt", expr: func(f *filterTarget) string { return f.compare("lt", f.beta) }, want: []int{0}},
	{op: "le", expr: func(f *filterTarget) string { return f.compare("le", f.beta) }, want: []int{0, 1}},
	{op: "not", expr: func(f *filterTarget) string { return "not (" + f.compare("eq", f.alpha) + ")" }, want: []int{1}},
}

func newFilterTable(t *testing.T, r *resource) *filterTable {
	t.Helper()

	table := &filterTable{r: r, targets: filterTargets(r.schema)}
	for _, target := range table.targets {
		table.seed(t, target)
	}
	return table
}

func (ft *filterTable) run(t *testing.T) {
	t.Helper()

	if len(ft.targets) == 0 {
		t.Skipf("n/a: %s has no readWrite singular string attribute", ft.r.kind.Name)
	}
	for _, tc := range filterCases {
		t.Run(tc.op, func(t *testing.T) {
			for _, target := range ft.targets {
				t.Run(target.path, func(t *testing.T) { ft.check(t, target, tc) })
			}
		})
	}
}

func (ft *filterTable) seed(t *testing.T, target *filterTarget) {
	t.Helper()

	target.unique = newUnique()
	target.alpha, target.beta = "scimtest alpha "+target.unique, "scimtest beta "+target.unique
	for _, value := range []string{target.alpha, target.beta} {
		body := ft.r.body(t, newUnique())
		setPath(body, target.path, value)
		_, created := ft.r.post(t, body)
		if created == nil {
			target.rejected = true
			return
		}
		target.ids = append(target.ids, idOf(created))
	}
}

func (ft *filterTable) check(t *testing.T, target *filterTarget, tc filterCase) {
	t.Helper()

	if target.rejected {
		t.Skipf("n/a: the server rejected %q as %s", target.alpha, target.path)
	}
	want := make([]string, 0, len(tc.want))
	if !tc.fold || !target.caseExact {
		for _, i := range tc.want {
			want = append(want, target.ids[i])
		}
	}
	filter := "(" + tc.expr(target) + ") and (" + idFilter(target.ids...) + ")"
	if got := ft.matches(t, filter, target.ids); !slices.Equal(got, want) {
		t.Errorf("scimtest: GET %s?filter=%s (RFC 7644 3.4.2.2)\n        want ids: %v\n        got ids:  %v", ft.r.kind.Endpoint, url.QueryEscape(filter), want, got)
	}
}

func (ft *filterTable) matches(t *testing.T, filter string, ours []string) []string {
	t.Helper()

	query := filterBy(filter)
	query.Set("count", strconv.Itoa(len(ours)))
	found := ft.r.filtered(t, query)
	return slices.DeleteFunc(slices.Clone(ours), func(id string) bool { return !slices.Contains(found, id) })
}

func (target *filterTarget) compare(op, value string) string {
	return target.path + " " + op + " " + quoted(value)
}

func filterTargets(schema *core.Schema) []*filterTarget {
	var targets []*filterTarget
	for _, attribute := range schema.Attributes {
		if isFilterable(attribute) {
			targets = append(targets, &filterTarget{path: attribute.Name, caseExact: attribute.CaseExact})
		}
		targets = append(targets, subTargets(attribute)...)
	}
	return targets
}

func subTargets(attribute *core.Attribute) []*filterTarget {
	if attribute.Type != core.TypeComplex || attribute.MultiValued || attribute.Mutability != core.MutabilityReadWrite {
		return nil
	}
	var targets []*filterTarget
	for _, sub := range attribute.SubAttributes {
		if isFilterable(sub) {
			targets = append(targets, &filterTarget{path: attribute.Name + "." + sub.Name, caseExact: sub.CaseExact})
		}
	}
	return targets
}

func isFilterable(attribute *core.Attribute) bool {
	return attribute.Type == core.TypeString && !attribute.MultiValued &&
		attribute.Mutability == core.MutabilityReadWrite && len(attribute.CanonicalValues) == 0 &&
		(attribute.Returned == core.ReturnedDefault || attribute.Returned == core.ReturnedAlways)
}

func setPath(body core.Object, path, value string) {
	name, sub, ok := strings.Cut(path, ".")
	if !ok {
		body.Set(name, value)
		return
	}
	parent, _ := body.Get(name).(map[string]any)
	if parent == nil {
		parent = map[string]any{}
	}
	parent[sub] = value
	body.Set(name, parent)
}
