package patch_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
)

func BenchmarkApplyTopLevelReplace(b *testing.B) {
	schemas := userSchemas()
	doc := map[string]any{"userName": "old"}
	ops := []patch.Operation{operation(patch.OpReplace, "userName", `"new"`)}

	benchApply(b, doc, ops, schemas)
}

func BenchmarkApplyValuePathSub(b *testing.B) {
	schemas := userSchemas()
	doc := map[string]any{"emails": []any{
		map[string]any{"type": "work", "value": "old@x"},
		map[string]any{"type": "home", "value": "h@x"},
	}}
	ops := []patch.Operation{operation(patch.OpReplace, `emails[type eq "work"].value`, `"new@x"`)}

	benchApply(b, doc, ops, schemas)
}

func BenchmarkApplyValuePathMerge(b *testing.B) {
	schemas := userSchemas()
	doc := map[string]any{"emails": []any{
		map[string]any{"type": "work", "value": "w@x"},
		map[string]any{"type": "home", "value": "h@x"},
	}}
	ops := []patch.Operation{operation(patch.OpReplace, `emails[type eq "work"]`, `{"value":"z@x"}`)}

	benchApply(b, doc, ops, schemas)
}

func BenchmarkApplyNoPathMerge(b *testing.B) {
	schemas := userSchemas()
	doc := map[string]any{"userName": "old", "displayName": "old"}
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
				benchApply(b, map[string]any{}, ops, c.schemas)
			})
		}
	}
}

func benchApply(b *testing.B, doc map[string]any, ops []patch.Operation, schemas []*core.Schema) {
	b.Helper()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := patch.Apply(doc, ops, schemas); err != nil {
			b.Fatal(err)
		}
	}
}
