package patch

import (
	"github.com/supabase-community/scim-go/internal/value"
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
	clauses   int
	matched   []bool
	budget    *budget
}

func (t *target) write(kind Op, value any) error {
	if _, ok := value.(map[string]any); t.key() == "" && !ok {
		return scimerrors.ErrInvalidValue(`"value" must be an object when "path" has no sub-attribute`)
	}
	if err := eachSub(t.owner(), value, gateWrite); err != nil {
		return err
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
		merge(holder, values, t.parent, kind)
		return nil
	case isObject && t.attr.Type == core.TypeComplex && !t.attr.MultiValued:
		nested, err := child(holder, t.key())
		if err != nil {
			return err
		}
		merge(nested, values, t.attr, kind)
		return nil
	}
	if err := t.overwritable(holder, kind); err != nil {
		return err
	}
	set(holder, t.key(), shaped(value, t.attr.MultiValued), kind)
	return nil
}

// RFC 7644 Section 3.5.2.2: a value that becomes unassigned and is read-only SHALL return "mutability".
func (t *target) overwritable(holder core.Object, kind Op) error {
	before := holder.Get(t.key())
	if _, appends := before.([]any); appends && kind == OpAdd {
		return nil
	}
	return eachSub(t.attr, before, gateRemove)
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
		if err := eachSub(t.attr, holder.Get(t.key()), gateRemove); err != nil {
			return err
		}
	}
	for _, holder := range holders {
		holder.Remove(t.key())
	}
	t.unassignEmptyExtension()
	return nil
}

// RFC 7644 Section 3.5.2.2: if no other values remain after removal, the attribute SHALL be considered unassigned.
func (t *target) unassignEmptyExtension() {
	if container, _ := t.container(false); t.extension != "" && len(container) == 0 {
		t.root.Remove(t.extension)
	}
}

func (t *target) drop() error {
	container, _ := t.container(false)
	elements := t.elements()
	matched, err := t.matches(elements)
	if err != nil {
		return err
	}
	kept := make([]any, 0, len(elements))
	for i, element := range elements {
		if !matched[i] {
			kept = append(kept, element)
			continue
		}
		if err := eachSub(t.attr, element, gateRemove); err != nil {
			return err
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
	elements, _ := value.([]any)
	switch {
	case t.match != nil:
		if t.matched, err = t.matches(elements); err != nil {
			return nil, err
		}
		return members(elements, t.matched)
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
		return members(elements, nil)
	}
	return nil, nil
}

func (t *target) matches(elements []any) ([]bool, error) {
	if err := t.budget.charge(len(elements) * t.clauses); err != nil {
		return nil, err
	}
	matched := make([]bool, len(elements))
	for i, element := range elements {
		member, ok := element.(map[string]any)
		matched[i] = ok && t.match(member)
	}
	return matched, nil
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
		return func(i int) bool { return t.matched[i] }
	case kind == OpAdd && t.path.SubAttribute == "":
		return func(i int) bool { return i >= len(elements) }
	}
	return func(int) bool { return true }
}

func (t *target) owner() *core.Attribute {
	if t.key() == "" {
		return t.parent
	}
	return t.attr
}

func (t *target) key() string {
	if t.match != nil || t.path.SubAttribute != "" {
		return t.path.SubAttribute
	}
	return t.path.Name
}

func members(elements []any, matched []bool) ([]core.Object, error) {
	holders := []core.Object{}
	for i, element := range elements {
		if member, ok := element.(map[string]any); ok && (matched == nil || matched[i]) {
			holders = append(holders, member)
		}
	}
	if len(holders) == 0 {
		return nil, scimerrors.ErrNoTarget(`"path" matched no elements`)
	}
	return holders, nil
}

// RFC 7644 Section 3.5.2.3: sub-attributes that are not specified in the "value" parameter are left unchanged.
func merge(holder core.Object, values map[string]any, parent *core.Attribute, kind Op) {
	keys := newKeys(holder)
	for name, value := range values {
		key := keys.resolve(name)
		holder[key] = appended(holder[key], shaped(value, subAttr(parent, name).MultiValued), kind)
		keys.add(key)
	}
}

func eachSub(attr *core.Attribute, value any, visit func(sub *core.Attribute, held any) error) error {
	switch v := value.(type) {
	case map[string]any:
		for name, held := range v {
			if err := visit(subAttr(attr, name), held); err != nil {
				return err
			}
		}
	case []any:
		for _, element := range v {
			if err := eachSub(attr, element, visit); err != nil {
				return err
			}
		}
	}
	return nil
}

// RFC 7644 Section 3.5.2: each operation against an attribute MUST be compatible with the attribute's mutability.
func gateWrite(sub *core.Attribute, _ any) error {
	return gate(sub)
}

// RFC 7644 Section 3.5.2.2: removing the value of a read-only attribute SHALL return "mutability".
func gateRemove(sub *core.Attribute, held any) error {
	if value.IsUnassigned(held) {
		return nil
	}
	return gate(sub)
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
