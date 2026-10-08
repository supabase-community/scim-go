package patch

import (
	"slices"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
)

// RFC 7644 Section 3.5.2: a client MUST NOT modify an attribute that has mutability "immutable".
func carryImmutable(attr *core.Attribute, kind Op, stored []any, candidate any) any {
	elements, ok := candidate.([]any)
	if kind != OpReplace || !ok {
		return candidate
	}
	subs := slices.DeleteFunc(slices.Clone(attr.SubAttributes), func(sub *core.Attribute) bool { return sub.Mutability != core.MutabilityImmutable })
	if len(subs) == 0 {
		return candidate
	}
	byIdentity := value.ByIdentity(attr, stored)
	for _, element := range elements {
		fillOmitted(attr, value.AsObject(element), subs, byIdentity)
	}
	return candidate
}

func fillOmitted(attr *core.Attribute, member map[string]any, subs []*core.Attribute, stored map[string]map[string]any) {
	if member == nil {
		return
	}
	existing := stored[value.Identity(attr, member)]
	if existing == nil {
		return
	}
	candidate, held := core.Object(member), core.Object(existing)
	for _, sub := range subs {
		if !candidate.Has(sub.Name) && held.Has(sub.Name) {
			candidate.Set(sub.Name, held.Get(sub.Name))
		}
	}
}
