package patch

import (
	"github.com/supabase-community/scim-go/pkg/core"
)

// RFC 7644 Section 3.5.2: setting "primary" to true sets it to false for every other value of the attribute.
func demote(elements []any, written func(int) bool) {
	promoted := false
	for i, element := range elements {
		promoted = promoted || written(i) && isPrimary(element)
	}
	for i, element := range elements {
		if promoted && !written(i) && isPrimary(element) {
			core.Object(element.(map[string]any)).Set("primary", false)
		}
	}
}

func isPrimary(element any) bool {
	member, _ := element.(map[string]any)
	primary, _ := core.Object(member).Get("primary").(bool)
	return primary
}
