package patch

import (
	"reflect"
	"slices"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
)

type indexKey struct {
	container uintptr
	name      string
}

// indexes holds the dedup index of every multi-valued attribute a patcher run has touched, reused across every op instead of rebuilt from its existing elements each time.
type indexes map[indexKey]*dedupIndex

// dedupIndex is the identity/set index of one multi-valued attribute.
type dedupIndex struct {
	attr    *core.Attribute
	set     *value.Set
	byValue map[string]map[string]any
}

func newDedupIndex(attr *core.Attribute, existing []any) *dedupIndex {
	if attr.SubAttribute("value") == nil {
		return &dedupIndex{attr: attr, set: value.NewSet(attr, existing)}
	}
	return &dedupIndex{attr: attr, byValue: value.ByIdentity(attr, existing)}
}

func (d *dedupIndex) contains(candidate any) bool {
	if d.set != nil {
		return d.set.Contains(candidate)
	}
	_, exists := d.byValue[value.Identity(d.attr, asMember(candidate))]
	return exists
}

func (d *dedupIndex) insert(added []any) {
	for _, candidate := range added {
		if d.set != nil {
			d.set.Insert(candidate)
			continue
		}
		if member := asMember(candidate); member != nil {
			if id := value.Identity(d.attr, member); id != "" {
				d.byValue[id] = member
			}
		}
	}
}

// fresh drops candidate elements already present at holder[key], per RFC 7644, Section 3.5.2.1: "If the target location already contains the value specified, no changes SHOULD be made". Elements identified by a "value" sub-attribute are matched by that identity, per RFC 7643 Section 2.4.
func (ix indexes) fresh(holder core.Object, key string, attr *core.Attribute, candidate []any) []any {
	idx := ix.indexFor(holder, key, attr)
	survivors := slices.DeleteFunc(slices.Clone(candidate), idx.contains)
	idx.insert(survivors)
	return survivors
}

func (ix indexes) indexFor(holder core.Object, key string, attr *core.Attribute) *dedupIndex {
	k := indexKeyFor(holder, key)
	if idx, ok := ix[k]; ok {
		return idx
	}
	existing, _ := holder.Get(key).([]any)
	idx := newDedupIndex(attr, existing)
	ix[k] = idx
	return idx
}

// invalidate drops a cached index once a write other than a dedup-checked add changes what holder[key] already contains.
func (ix indexes) invalidate(holder core.Object, key string) {
	delete(ix, indexKeyFor(holder, key))
}

func indexKeyFor(holder core.Object, name string) indexKey {
	return indexKey{container: reflect.ValueOf(map[string]any(holder)).Pointer(), name: name}
}
