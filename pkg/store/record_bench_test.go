package store_test

import (
	"encoding/json"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/store"
)

func BenchmarkRecordInto(b *testing.B) {
	members := store.ReferencesOf(groupSchemas())[0]
	relations := make([]store.Relation, 0, 10)
	for range 10 {
		relations = append(relations, store.Relation{Reference: members, ID: memberID, Type: "User"})
	}
	record := store.Record{ID: "1", Body: store.Embed(json.RawMessage(`{"displayName":"Admins"}`), relations)}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := record.Into[*core.Group](); err != nil {
			b.Fatal(err)
		}
	}
}
