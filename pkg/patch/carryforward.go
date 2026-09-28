package patch

import (
	"slices"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
)

// RFC 7644 Section 3.5.2: an immutable sub-attribute a replacement element omits keeps its stored value, matched by "value" to the element it replaces.
func carryImmutable(attr *core.Attribute, kind Op, stored []any, candidate any) any {
	elements, ok := candidate.([]any)
	subs := immutableSubs(attr.SubAttributes)
	if kind != OpReplace || !ok || attr.SubAttribute("value") == nil || len(subs) == 0 {
		return candidate
	}
	byKey := byValue(stored)
	for _, element := range elements {
		fillOmitted(asMember(element), subs, byKey)
	}
	return candidate
}

func immutableSubs(subs []*core.Attribute) []*core.Attribute {
	return slices.DeleteFunc(slices.Clone(subs), func(sub *core.Attribute) bool { return sub.Mutability != core.MutabilityImmutable })
}

func byValue(elements []any) map[string]map[string]any {
	stored := map[string]map[string]any{}
	for _, element := range elements {
		if member := asMember(element); member != nil {
			if key, ok := value.Key(member); ok {
				stored[key] = member
			}
		}
	}
	return stored
}

func fillOmitted(member map[string]any, subs []*core.Attribute, stored map[string]map[string]any) {
	if member == nil {
		return
	}
	key, ok := value.Key(member)
	existing := stored[key]
	if !ok || existing == nil {
		return
	}
	for _, sub := range subs {
		if _, present := member[sub.Name]; !present {
			if held, has := existing[sub.Name]; has {
				member[sub.Name] = held
			}
		}
	}
}

func asMember(element any) map[string]any {
	member, _ := element.(map[string]any)
	return member
}
