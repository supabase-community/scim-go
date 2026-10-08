package patch

import (
	"slices"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
)

// RFC 7644 Section 3.5.2.1: if the target location already contains the value specified, no changes SHOULD be made.
type index struct {
	set       *value.Set
	size      int
	primaries []any
}

type indexCache map[string]*index

func newIndex(attr *core.Attribute, existing []any) *index {
	ix := &index{set: value.NewSet(attr, existing), size: len(existing)}
	for _, element := range existing {
		if value.Primary(element) {
			ix.primaries = append(ix.primaries, element)
		}
	}
	return ix
}

func (ix *index) fresh(candidates []any) []any {
	return slices.DeleteFunc(slices.Clone(candidates), ix.set.Contains)
}

func (ix *index) add(stored []any) {
	ix.size += len(stored)
	ix.set.Add(stored...)
}

func (c indexCache) lookup(extension, name string, attr *core.Attribute, elements []any) *index {
	key := extension + ":" + name
	if ix, ok := c[key]; ok && ix.size == len(elements) {
		return ix
	}
	c[key] = newIndex(attr, elements)
	return c[key]
}
