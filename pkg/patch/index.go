package patch

import (
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
	if keyedByIdentity(attr) {
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

func (ix *index) fresh(stored, candidates []any) ([]any, error) {
	kept := make([]any, 0, len(candidates))
	for _, candidate := range candidates {
		switch {
		case !ix.contains(candidate):
			kept = append(kept, candidate)
		case value.Primary(candidate) && ix.attr.SubAttribute("primary") != nil:
			if err := ix.promote(stored, candidate); err != nil {
				return nil, err
			}
		}
	}
	return kept, nil
}

// RFC 7644 Section 3.5.2: setting "primary" to "true" sets it to "false" for every other value of the attribute.
func (ix *index) promote(stored []any, candidate any) error {
	promoted, flips := ix.flips(stored, value.Identity(ix.attr, asMember(candidate)))
	if len(flips) > 0 && ix.attr.Mutability == core.MutabilityImmutable {
		return errImmutable(ix.attr)
	}
	ix.flip(stored, promoted, flips)
	return nil
}

func (ix *index) flip(stored []any, promoted int, flips []int) {
	for _, i := range flips {
		setPrimary(stored[i], i == promoted)
	}
	ix.primaries = ix.primaries[:0]
	if promoted >= 0 {
		ix.primaries = append(ix.primaries, stored[promoted])
	}
}

func (ix *index) flips(stored []any, id string) (promoted int, flips []int) {
	promoted = -1
	for i, element := range stored {
		primary := promoted < 0 && value.Identity(ix.attr, asMember(element)) == id
		if primary {
			promoted = i
		}
		if value.Primary(element) != primary {
			flips = append(flips, i)
		}
	}
	return promoted, flips
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

func keyedByIdentity(attr *core.Attribute) bool {
	return len(attr.SubAttributes) > 0
}
