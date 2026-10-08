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
	set := value.NewSet(attr, stored)
	for _, element := range elements {
		fillOmitted(value.AsObject(element), subs, set)
	}
	return candidate
}

func fillOmitted(member core.Object, subs []*core.Attribute, stored *value.Set) {
	if member == nil {
		return
	}
	existing := stored.Find(member)
	if existing == nil {
		return
	}
	for _, sub := range subs {
		if !member.Has(sub.Name) && existing.Has(sub.Name) {
			member.Set(sub.Name, existing.Get(sub.Name))
		}
	}
}
