package patch

import "strings"

// RFC 7643 2.1 - attribute names are case insensitive.
type object map[string]any

func (o object) key(name string) string {
	if _, ok := o[name]; ok {
		return name
	}
	canonical := ""
	for candidate := range o {
		if !strings.EqualFold(candidate, name) {
			continue
		}
		if canonical == "" || candidate < canonical {
			canonical = candidate
		}
	}
	if canonical == "" {
		return name
	}
	return canonical
}

func (o object) get(name string) (any, bool) {
	value, ok := o[o.key(name)]
	return value, ok
}

func (o object) has(name string) bool {
	_, ok := o[o.key(name)]
	return ok
}
