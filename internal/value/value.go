package value

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
