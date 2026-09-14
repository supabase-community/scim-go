package patch

import (
	"strings"

	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

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

func (o object) set(name string, value any, appendMode bool) {
	key := o.key(name)
	if before, ok := o[key].([]any); appendMode && ok {
		if list, ok := value.([]any); ok {
			o[key] = append(before, list...)
		} else {
			o[key] = append(before, value)
		}
		return
	}
	o[key] = value
}

func (o object) remove(name string) {
	delete(o, o.key(name))
}

func (o object) child(name string) (object, error) {
	key := o.key(name)
	if existing := o[key]; existing != nil {
		nested, ok := existing.(map[string]any)
		if !ok {
			return nil, scimerrors.ErrInvalidPath(`"path" targets a non-complex attribute`)
		}
		return nested, nil
	}
	fresh := map[string]any{}
	o[key] = fresh
	return fresh, nil
}
