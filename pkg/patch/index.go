package patch

import (
	"slices"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
)

// RFC 7644 Section 3.5.2.1: if the target location already contains the value specified, no changes SHOULD be made.
type index struct {
	attr      *core.Attribute
	set       *value.Set
	ids       map[string]bool
	size      int
	primaries []any
}

type indexCache map[string]*index

func newIndex(attr *core.Attribute, existing []any) *index {
	ix := &index{attr: attr}
	if keyedByValue(attr) {
		ix.ids = make(map[string]bool, len(existing))
	} else {
		ix.set = value.NewSet(attr, nil)
	}
	ix.add(existing)
	for _, element := range existing {
		if value.Primary(element) {
			ix.primaries = append(ix.primaries, element)
		}
	}
	return ix
}

func (ix *index) fresh(candidates []any) []any {
	return slices.DeleteFunc(slices.Clone(candidates), ix.contains)
}

func (ix *index) add(stored []any) {
	ix.size += len(stored)
	if ix.set != nil {
		ix.set.Add(stored...)
		return
	}
	for _, element := range stored {
		if member, ok := element.(map[string]any); ok {
			ix.ids[value.Identity(ix.attr, member)] = true
		}
	}
}

func (ix *index) contains(candidate any) bool {
	if ix.set != nil {
		return ix.set.Contains(candidate)
	}
	id := value.Identity(ix.attr, asMember(candidate))
	return id != "" && ix.ids[id]
}

func (c indexCache) lookup(extension, name string, attr *core.Attribute, elements []any) *index {
	key := extension + ":" + name
	if ix, ok := c[key]; ok && ix.size == len(elements) {
		return ix
	}
	c[key] = newIndex(attr, elements)
	return c[key]
}

func keyedByValue(attr *core.Attribute) bool {
	return attr.SubAttribute("value") != nil
}
