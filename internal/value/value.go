package value

import "github.com/supabase-community/scim-go/pkg/core"

// IsUnassigned reports nil, "", [] or {}; RFC 7643 Section 2.5: null or an empty array SHALL be considered equivalent to unassigned.
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

// Key returns the "value" sub-attribute that identifies an element of a multi-valued attribute, per RFC 7643, Section 2.4.
func Key(element core.Object) (string, bool) {
	key, ok := element.Get("value").(string)
	return key, ok && key != ""
}

func Primary(element any) bool {
	switch e := element.(type) {
	case core.Object:
		return e.Get("primary") == true
	case map[string]any:
		return core.Object(e).Get("primary") == true
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
