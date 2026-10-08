package value

import "github.com/supabase-community/scim-go/pkg/core"

// IsUnassigned reports nil, "", [] or {}, the values that RFC 7644 Section 3.4.2.2 "pr" treats as not present.
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

// AllUnassigned reports an object whose every value is unassigned, which RFC 7643 Section 2.5 treats as holding no data.
func AllUnassigned(object map[string]any) bool {
	for _, held := range object {
		if nested, ok := held.(map[string]any); ok && AllUnassigned(nested) {
			continue
		}
		if !IsUnassigned(held) {
			return false
		}
	}
	return true
}

// Key returns the "value" sub-attribute that identifies an element of a multi-valued attribute, per RFC 7643, Section 2.4.
func Key(element core.Object) (string, bool) {
	key, ok := element.Get("value").(string)
	return key, ok && key != ""
}

func Clone(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, element := range typed {
			cloned[key] = Clone(element)
		}
		return cloned
	case []any:
		cloned := make([]any, len(typed))
		for i, element := range typed {
			cloned[i] = Clone(element)
		}
		return cloned
	default:
		return typed
	}
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
