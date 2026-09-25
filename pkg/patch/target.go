package patch

import (
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type target struct {
	root      core.Object
	extension string
	parent    *core.Attribute
	attr      *core.Attribute
	path      filter.Path
	match     predicate
}

func (t *target) write(kind Op, value any) error {
	if _, ok := value.(map[string]any); t.key() == "" && !ok {
		return scimerrors.ErrInvalidValue(`"value" must be an object when "path" has no sub-attribute`)
	}
	holders, err := t.holders(true)
	if err != nil {
		return err
	}
	written := t.written(t.elements(), kind)
	for _, holder := range holders {
		if err := t.put(holder, kind, value); err != nil {
			return err
		}
	}
	if promotes(t.key(), value) {
		demote(t.elements(), written)
	}
	return nil
}

func (t *target) put(holder core.Object, kind Op, value any) error {
	values, isObject := value.(map[string]any)
	switch {
	case t.key() == "":
		return merge(holder, values, t.parent, kind)
	case isObject && t.attr.Type == core.TypeComplex && !t.attr.MultiValued:
		nested, err := child(holder, t.key())
		if err != nil {
			return err
		}
		return merge(nested, values, t.attr, kind)
	}
	set(holder, t.key(), shaped(value, t.attr.MultiValued), kind)
	return nil
}

func (t *target) remove() error {
	if t.match != nil && t.path.SubAttribute == "" {
		return t.drop()
	}
	holders, err := t.holders(false)
	if err != nil {
		return err
	}
	for _, holder := range holders {
		holder.Remove(t.key())
	}
	return nil
}

func (t *target) drop() error {
	container, _ := t.container(false)
	elements := t.elements()
	kept := make([]any, 0, len(elements))
	for _, element := range elements {
		if member, ok := element.(map[string]any); !ok || !t.match(member) {
			kept = append(kept, element)
		}
	}
	if len(kept) == len(elements) {
		return scimerrors.ErrNoTarget(`"path" matched no elements`)
	}
	container.Set(t.path.Name, kept)
	return nil
}

func (t *target) holders(create bool) ([]core.Object, error) {
	container, err := t.container(create)
	if err != nil {
		return nil, err
	}
	value := container.Get(t.path.Name)
	switch {
	case t.match != nil:
		return matching(value, t.match)
	case t.path.SubAttribute == "":
		return []core.Object{container}, nil
	case create:
		nested, err := child(container, t.path.Name)
		return []core.Object{nested}, err
	}
	switch typed := value.(type) {
	case map[string]any:
		return []core.Object{typed}, nil
	case []any:
		return matching(value, func(map[string]any) bool { return true })
	}
	return nil, nil
}

func (t *target) container(create bool) (core.Object, error) {
	switch {
	case t.extension == "":
		return t.root, nil
	case create:
		return child(t.root, t.extension)
	}
	nested, _ := t.root.Get(t.extension).(map[string]any)
	return nested, nil
}

func (t *target) elements() []any {
	container, _ := t.container(false)
	elements, _ := container.Get(t.path.Name).([]any)
	return elements
}

func (t *target) written(elements []any, kind Op) func(int) bool {
	switch {
	case t.match != nil:
		matched := make([]bool, len(elements))
		for i, element := range elements {
			member, ok := element.(map[string]any)
			matched[i] = ok && t.match(member)
		}
		return func(i int) bool { return i < len(matched) && matched[i] }
	case kind == OpAdd && t.path.SubAttribute == "":
		return func(i int) bool { return i >= len(elements) }
	}
	return func(int) bool { return true }
}

func (t *target) key() string {
	if t.match != nil || t.path.SubAttribute != "" {
		return t.path.SubAttribute
	}
	return t.path.Name
}

func matching(value any, match predicate) ([]core.Object, error) {
	elements, _ := value.([]any)
	matched := []core.Object{}
	for _, element := range elements {
		if member, ok := element.(map[string]any); ok && match(member) {
			matched = append(matched, member)
		}
	}
	if len(matched) == 0 {
		return nil, scimerrors.ErrNoTarget(`"path" matched no elements`)
	}
	return matched, nil
}

// RFC 7644 Section 3.5.2.3: sub-attributes that are not specified in the "value" parameter are left unchanged.
func merge(holder core.Object, values map[string]any, parent *core.Attribute, kind Op) error {
	keys := newKeys(holder)
	for name, value := range values {
		attr := subAttr(parent, name)
		if err := gate(attr); err != nil {
			return err
		}
		key := keys.resolve(name)
		holder[key] = appended(holder[key], shaped(value, attr.MultiValued), kind)
		keys.add(key)
	}
	return nil
}

func subAttr(parent *core.Attribute, name string) *core.Attribute {
	if sub := parent.SubAttribute(name); sub != nil {
		return sub
	}
	return permissiveAttr
}

// RFC 7644 3.5.2.1 - a value written to a multi-valued attribute is an array.
func shaped(value any, multiValued bool) any {
	if !multiValued {
		return value
	}
	if list, ok := value.([]any); ok {
		return list
	}
	return []any{value}
}

func set(o core.Object, name string, value any, kind Op) {
	o.Set(name, appended(o.Get(name), value, kind))
}

func appended(before, value any, kind Op) any {
	if list, ok := before.([]any); kind == OpAdd && ok {
		return append(list, shaped(value, true).([]any)...)
	}
	return value
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
