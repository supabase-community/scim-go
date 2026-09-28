package value

import (
	"encoding/json"

	"github.com/supabase-community/scim-go/pkg/core"
)

// Identity returns the string that identifies element within a multi-valued attribute, per RFC 7643 Section 2.4: "value" identifies it when the attribute declares one; otherwise the client-visible sub-attributes it can see and write do.
func Identity(attribute *core.Attribute, element core.Object) string {
	if sub := attribute.SubAttribute("value"); sub != nil {
		key, _ := Key(element)
		folded, _ := Fold(sub, key).(string)
		return folded
	}
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

// ByIdentity indexes the elements of a multi-valued attribute by Identity; the first element with a given identity wins.
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
