package patch

import (
	"slices"
	"strings"

	"github.com/supabase-community/scim-go/internal/value"
)

// RFC 7644 Section 3.5.2: setting "primary" to true sets it to false for every other value of the attribute.
func demote(elements []any, written func(int) bool) {
	for i, element := range elements {
		if written(i) || !value.Primary(element) {
			continue
		}
		setPrimary(element, false)
	}
}

func setPrimary(element any, primary bool) {
	if object := value.AsObject(element); object != nil {
		object.Set("primary", primary)
	}
}

func promotes(key string, written any) bool {
	if strings.EqualFold(key, "primary") {
		return written == true
	}
	list, _ := written.([]any)
	return value.Primary(written) || slices.ContainsFunc(list, value.Primary)
}
