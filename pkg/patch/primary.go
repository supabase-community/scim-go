package patch

import (
	"slices"
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
)

// RFC 7644 Section 3.5.2: setting "primary" to true sets it to false for every other value of the attribute.
func demote(elements []any, written func(int) bool) {
	for i, element := range elements {
		if !written(i) && isPrimary(element) {
			core.Object(element.(map[string]any)).Set("primary", false)
		}
	}
}

func promotes(key string, value any) bool {
	if strings.EqualFold(key, "primary") {
		return value == true
	}
	list, _ := value.([]any)
	return isPrimary(value) || slices.ContainsFunc(list, isPrimary)
}

func isPrimary(element any) bool {
	member, _ := element.(map[string]any)
	primary, _ := core.Object(member).Get("primary").(bool)
	return primary
}
