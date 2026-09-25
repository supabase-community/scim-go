package value

import "github.com/supabase-community/scim-go/pkg/core"

// IsUnassigned reports a null or empty value, equivalent in state to an unassigned attribute, per RFC 7643, Section 2.5.
func IsUnassigned(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return v == ""
	case []any:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	}
	return false
}

// Hidden reports an attribute, or the parent it belongs to, whose values SHALL NOT be returned, per RFC 7643, Section 7.
func Hidden(parent, attribute *core.Attribute) bool {
	return hides(parent) || hides(attribute)
}

func hides(attribute *core.Attribute) bool {
	return attribute != nil && (attribute.Mutability == core.MutabilityWriteOnly || attribute.Returned == core.ReturnedNever)
}
