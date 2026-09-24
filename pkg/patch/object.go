package patch

import (
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

func set(o core.Object, name string, value any, appendMode bool) {
	if before, ok := o.Get(name).([]any); appendMode && ok {
		o.Set(name, append(before, shaped(value, true).([]any)...))
		return
	}
	o.Set(name, value)
}

func child(o core.Object, name string) (core.Object, error) {
	existing := o.Get(name)
	if existing == nil {
		fresh := map[string]any{}
		o.Set(name, fresh)
		return fresh, nil
	}
	nested, ok := existing.(map[string]any)
	if !ok {
		return nil, scimerrors.ErrInvalidPath(`"path" targets a non-complex attribute`)
	}
	return nested, nil
}
