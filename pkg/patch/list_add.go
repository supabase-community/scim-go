package patch

import (
	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
)

type listAdd struct {
	t *target
}

func (r listAdd) demoteIfPromoted(kind Op, before []any, written func(int) bool, incoming any) {
	switch {
	case !promotes(r.t.key(), incoming):
		return
	case r.isAdd(kind):
		r.demotePrimaries(before)
	default:
		demote(r.t.elements(), written)
	}
}

func (r listAdd) demotePrimaries(before []any) {
	ix := r.index(before)
	for _, held := range ix.primaries {
		setPrimary(held, false)
	}
	after := r.t.elements()
	ix.primaries = ix.primaries[:0]
	for _, element := range after[len(before):] {
		if value.Primary(element) {
			ix.primaries = append(ix.primaries, element)
		}
	}
}

// RFC 7644 Section 3.5.2.1: if the target location already contains the value specified, no changes SHOULD be made.
func (r listAdd) dedupedAdd(holder core.Object, kind Op, shapedValue any) any {
	existing, ok := holder.Get(r.t.key()).([]any)
	if kind != OpAdd || !r.t.attr.MultiValued || !ok || r.isAdd(kind) {
		return shapedValue
	}
	return newIndex(r.t.attr, existing).fresh(shapedList(shapedValue))
}

func (r listAdd) isAdd(kind Op) bool {
	return kind == OpAdd && r.t.filter.match == nil && r.t.path.Name != "" && r.t.path.SubAttribute == "" && r.t.attr.MultiValued
}

func (r listAdd) fresh(kind Op, candidate any) any {
	if !r.isAdd(kind) {
		return candidate
	}
	return r.index(r.t.elements()).fresh(shapedList(candidate))
}

func (r listAdd) record(kind Op, before []any, added any) {
	if !r.isAdd(kind) || promotes(r.t.key(), added) && !value.KeyedByIdentity(r.t.attr) {
		clear(r.t.indexes)
		return
	}
	r.index(before).add(r.t.elements()[len(before):])
}

func (r listAdd) index(elements []any) *index {
	return r.t.indexes.lookup(r.t.extension, r.t.path.Name, r.t.attr, elements)
}
