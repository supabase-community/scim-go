package value

import (
	"encoding/json"

	"github.com/supabase-community/scim-go/pkg/core"
)

// Identity returns the key of element within a multi-valued attribute; RFC 7643 Section 2.4: the same "value" MAY repeat with a different "type".
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
		if sub.Mutability == core.MutabilityReadWrite && !Hidden(nil, sub) {
			folded = append(folded, Fold(sub, element.Get(sub.Name)))
		}
	}
	if len(folded) == 0 {
		return ""
	}
	raw, _ := json.Marshal(folded)
	return string(raw)
}
