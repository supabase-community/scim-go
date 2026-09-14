package patch_test

import (
	"encoding/json"
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

func benchApply(b *testing.B, doc map[string]any, ops []patch.Operation, schemas []*core.Schema) {
	b.Helper()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := patch.Apply(doc, ops, schemas); err != nil {
			b.Fatal(err)
		}
	}
}
