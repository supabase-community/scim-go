package scimtest

import (
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
	set       func(body core.Object, value string)
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

	table := &filterTable{r: r, targets: filterTargets(r.schemas)}
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
		target.set(body, value)
		body.Set("schemas", ft.r.schemaURIs(body))
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
	ft.r.expectMatches(t, tc.expr(target), target.ids, want)
}

func (target *filterTarget) compare(op, value string) string {
	return target.path + " " + op + " " + quoted(value)
}

func filterTargets(schemas core.Schemas) []*filterTarget {
	root := func(body core.Object) core.Object { return body }
	targets := attributeTargets(commonTargets(), "", root)
	targets = append(targets, attributeTargets(schemas.Base().Attributes, "", root)...)
	for _, extension := range schemas.Extensions() {
		uri := string(extension.ID)
		targets = append(targets, attributeTargets(extension.Attributes, uri+":", func(body core.Object) core.Object {
			holder, _ := body.Get(uri).(map[string]any)
			if holder == nil {
				holder = map[string]any{}
				body.Set(uri, holder)
			}
			return holder
		})...)
	}
	return targets
}

func commonTargets() core.Attributes {
	externalID, _ := core.CommonAttribute("externalId")
	return core.Attributes{externalID}
}

func attributeTargets(attributes core.Attributes, prefix string, holder func(core.Object) core.Object) []*filterTarget {
	var targets []*filterTarget
	for _, attribute := range attributes {
		if isFilterable(attribute) {
			targets = append(targets, &filterTarget{path: prefix + attribute.Name, caseExact: attribute.CaseExact, set: func(body core.Object, value string) {
				holder(body).Set(attribute.Name, value)
			}})
		}
		targets = append(targets, subTargets(attribute, prefix, holder)...)
	}
	return targets
}

func subTargets(attribute *core.Attribute, prefix string, holder func(core.Object) core.Object) []*filterTarget {
	if attribute.Type != core.TypeComplex || attribute.Mutability != core.MutabilityReadWrite {
		return nil
	}
	var targets []*filterTarget
	for _, sub := range attribute.SubAttributes {
		if isFilterable(sub) {
			targets = append(targets, &filterTarget{path: prefix + attribute.Name + "." + sub.Name, caseExact: sub.CaseExact, set: subSetter(holder, attribute, sub)})
		}
	}
	return targets
}

func subSetter(holder func(core.Object) core.Object, attribute, sub *core.Attribute) func(core.Object, string) {
	if attribute.MultiValued {
		return func(body core.Object, value string) {
			holder(body).Set(attribute.Name, []any{map[string]any{sub.Name: value}})
		}
	}
	return func(body core.Object, value string) {
		parent, _ := holder(body).Get(attribute.Name).(map[string]any)
		if parent == nil {
			parent = map[string]any{}
		}
		parent[sub.Name] = value
		holder(body).Set(attribute.Name, parent)
	}
}
