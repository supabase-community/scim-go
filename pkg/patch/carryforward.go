package patch

import (
	"slices"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
)

// RFC 7644 Section 3.5.2: an immutable sub-attribute a replacement element omits keeps its stored value, matched by identity to the element it replaces.
func carryImmutable(attr *core.Attribute, kind Op, stored []any, candidate any) any {
	elements, ok := candidate.([]any)
	subs := immutableSubs(attr.SubAttributes)
	if kind != OpReplace || !ok || len(subs) == 0 {
		return candidate
	}
	byIdentity := value.ByIdentity(attr, stored)
	for _, element := range elements {
		fillOmitted(attr, asMember(element), subs, byIdentity)
	}
	return candidate
}

func immutableSubs(subs []*core.Attribute) []*core.Attribute {
	return slices.DeleteFunc(slices.Clone(subs), func(sub *core.Attribute) bool { return sub.Mutability != core.MutabilityImmutable })
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

func asMember(element any) map[string]any {
	member, _ := element.(map[string]any)
	return member
}
