package value

import (
	"encoding/json"

	"github.com/supabase-community/scim-go/pkg/core"
)

// Identity returns the string that identifies element within a multi-valued attribute, per RFC 7643 Section 2.4: "value" identifies it, together with "type" when "type" is readWrite, since the same "value" MAY then repeat under a different "type"; without a "value" sub-attribute, the client-visible sub-attributes it can see and write do.
func Identity(attribute *core.Attribute, element core.Object) string {
	sub := attribute.SubAttribute("value")
	if sub == nil {
		return compositeIdentity(attribute, element)
	}
	key, ok := Key(element)
	if !ok {
		return ""
	}
	folded := Fold(sub, key)
	kind := attribute.SubAttribute("type")
	if kind == nil || kind.Mutability != core.MutabilityReadWrite {
		raw, _ := json.Marshal(folded)
		return string(raw)
	}
	typ := Fold(kind, element.Get(kind.Name))
	if IsUnassigned(typ) {
		typ = nil
	}
	raw, _ := json.Marshal([]any{folded, typ})
	return string(raw)
}

// ByIdentity indexes the elements of a multi-valued attribute by Identity, per RFC 7643, Section 2.4.
func ByIdentity(attribute *core.Attribute, existing any) map[string]map[string]any {
	elements, _ := existing.([]any)
	stored := make(map[string]map[string]any, len(elements))
	for _, element := range elements {
		object, ok := element.(map[string]any)
		if id := Identity(attribute, object); ok && id != "" && stored[id] == nil {
			stored[id] = object
		}
	}
	return stored
}

func compositeIdentity(attribute *core.Attribute, element core.Object) string {
	folded := []any{}
	for _, sub := range attribute.SubAttributes {
		if sub.Mutability != core.MutabilityReadOnly && !Hidden(nil, sub) {
			folded = append(folded, Fold(sub, element.Get(sub.Name)))
		}
	}
	if len(folded) == 0 {
		return ""
	}
	raw, _ := json.Marshal(folded)
	return string(raw)
}
