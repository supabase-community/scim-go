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
	case core.Object:
		return len(v) == 0
	}
	return false
}

// AllUnassigned reports an object whose every value is unassigned, which RFC 7643 Section 2.5 treats as holding no data.
func AllUnassigned(object core.Object) bool {
	for _, held := range object {
		if nested := AsObject(held); nested != nil && AllUnassigned(nested) {
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

func Clone(v any) any {
	return transform(v, func(leaf any) any { return leaf })
}

func AsObject(element any) core.Object {
	switch object := element.(type) {
	case core.Object:
		return object
	case map[string]any:
		return object
	}
	return nil
}

func Primary(element any) bool {
	return AsObject(element).Get("primary") == true
}

// Hidden reports an attribute, or the parent it belongs to, whose values SHALL NOT be returned, per RFC 7643, Section 7.
func Hidden(parent, attribute *core.Attribute) bool {
	return hides(parent) || hides(attribute)
}

func hides(attribute *core.Attribute) bool {
	return attribute != nil && (attribute.Mutability == core.MutabilityWriteOnly || attribute.Returned == core.ReturnedNever)
}

func transform(v any, leaf func(any) any) any {
	if object := AsObject(v); object != nil {
		out := make(map[string]any, len(object))
		for key, element := range object {
			out[key] = transform(element, leaf)
		}
		return out
	}
	if list, ok := v.([]any); ok {
		out := make([]any, len(list))
		for i, element := range list {
			out[i] = transform(element, leaf)
		}
		return out
	}
	return leaf(v)
}
