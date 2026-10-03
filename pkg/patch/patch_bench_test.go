package patch_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
)

func BenchmarkApplyTopLevelReplace(b *testing.B) {
	schemas := userSchemas()
	doc := func() map[string]any {
		return map[string]any{"userName": "old"}
	}
	ops := []patch.Operation{operation(patch.OpReplace, "userName", `"new"`)}

	benchApply(b, doc, ops, schemas)
}

func BenchmarkApplyValuePathSub(b *testing.B) {
	schemas := userSchemas()
	doc := func() map[string]any {
		return map[string]any{"emails": []any{
			map[string]any{"type": "work", "value": "old@x"},
			map[string]any{"type": "home", "value": "h@x"},
		}}
	}
	ops := []patch.Operation{operation(patch.OpReplace, `emails[type eq "work"].value`, `"new@x"`)}

	benchApply(b, doc, ops, schemas)
}

func BenchmarkApplyValuePathMerge(b *testing.B) {
	schemas := userSchemas()
	doc := func() map[string]any {
		return map[string]any{"emails": []any{
			map[string]any{"type": "work", "value": "w@x"},
			map[string]any{"type": "home", "value": "h@x"},
		}}
	}
	ops := []patch.Operation{operation(patch.OpReplace, `emails[type eq "work"]`, `{"value":"z@x"}`)}

	benchApply(b, doc, ops, schemas)
}

func BenchmarkApplyNoPathMerge(b *testing.B) {
	schemas := userSchemas()
	doc := func() map[string]any {
		return map[string]any{"userName": "old", "displayName": "old"}
	}
	ops := []patch.Operation{{
		Op:    patch.OpAdd,
		Value: json.RawMessage(`{"userName":"new","displayName":"new"}`),
	}}

	benchApply(b, doc, ops, schemas)
}

func BenchmarkApplyManyAdds(b *testing.B) {
	cases := []struct {
		name    string
		schemas []*core.Schema
		path    string
		value   string
	}{
		{"members", []*core.Schema{core.NewSchema(core.SchemaGroup).With(core.GroupAttributes()...)}, "members", `{"value":"%d"}`},
		{"emails", []*core.Schema{core.NewSchema(core.SchemaUser).With(core.UserAttributes()...)}, "emails", `{"value":"%d@x"}`},
		{"primary emails", []*core.Schema{core.NewSchema(core.SchemaUser).With(core.UserAttributes()...)}, "emails", `{"value":"%d@x","primary":true}`},
	}
	for _, c := range cases {
		for _, n := range []int{1000, 10000} {
			ops := make([]patch.Operation, n)
			for i := range ops {
				ops[i] = operation(patch.OpAdd, c.path, fmt.Sprintf(c.value, i))
			}
			b.Run(fmt.Sprintf("%s/ops=%d", c.name, n), func(b *testing.B) {
				benchApply(b, func() map[string]any { return map[string]any{} }, ops, c.schemas)
			})
		}
	}
}

func BenchmarkApplyManyAddsToALargeGroup(b *testing.B) {
	schemas := groupSchemas()
	ops := make([]patch.Operation, 100)
	for i := range ops {
		ops[i] = operation(patch.OpAdd, "members", fmt.Sprintf(`{"value":"new-%d"}`, i))
	}
	for _, n := range []int{1000, 10000} {
		members := make([]any, n)
		for i := range members {
			members[i] = map[string]any{"value": fmt.Sprintf("m-%d", i)}
		}
		b.Run(fmt.Sprintf("members=%d/ops=100", n), func(b *testing.B) {
			benchApply(b, func() map[string]any { return map[string]any{"members": slices.Clone(members)} }, ops, schemas)
		})
	}
}

func BenchmarkApplyValueFilterOnALargeGroup(b *testing.B) {
	schemas := groupSchemas()
	path := "members[" + strings.Repeat(`value eq "nope" or `, 99) + `value eq "m-0"]`
	ops := []patch.Operation{{Op: patch.OpRemove, Path: path}}
	for _, n := range []int{1000, 10000} {
		members := make([]any, n)
		for i := range members {
			members[i] = map[string]any{"value": fmt.Sprintf("m-%d", i)}
		}
		b.Run(fmt.Sprintf("members=%d/clauses=100", n), func(b *testing.B) {
			benchApply(b, func() map[string]any { return map[string]any{"members": slices.Clone(members)} }, ops, schemas)
		})
	}
}

func BenchmarkApplyManyAddsWithoutAValueSubAttribute(b *testing.B) {
	schemas := []*core.Schema{core.NewSchema(core.SchemaUser).With(core.UserAttributes()...)}
	for _, n := range []int{1000, 10000} {
		first := make([]map[string]any, n)
		second := make([]map[string]any, n)
		for i := range n {
			first[i] = map[string]any{"postalCode": fmt.Sprintf("a%d", i)}
			second[i] = map[string]any{"postalCode": fmt.Sprintf("b%d", i)}
		}
		firstValue, _ := json.Marshal(first)
		secondValue, _ := json.Marshal(second)
		ops := []patch.Operation{
			{Op: patch.OpAdd, Path: "addresses", Value: firstValue},
			{Op: patch.OpAdd, Path: "addresses", Value: secondValue},
		}
		b.Run(fmt.Sprintf("ops=%d", n), func(b *testing.B) {
			benchApply(b, func() map[string]any { return map[string]any{} }, ops, schemas)
		})
	}
}

func benchApply(b *testing.B, doc func() map[string]any, ops []patch.Operation, schemas []*core.Schema) {
	b.Helper()
	b.ReportAllocs()
	for b.Loop() {
		if err := patch.Apply(doc(), ops, schemas); err != nil {
			b.Fatal(err)
		}
	}
}
