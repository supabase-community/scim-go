package patch

import (
	"strconv"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// merger holds the state one merge shares across its fields, per RFC 7644 Section 3.5.2.3.
type merger struct {
	holder core.Object
	parent *core.Attribute
	kind   Op
	gate   parentGate
}

// parentGate reports whether a sub-attribute write also changes its immutable parent complex attribute, per RFC 7643 Section 7.
type parentGate struct {
	attr     *core.Attribute
	assigned bool
}

func newMerger(holder core.Object, parent *core.Attribute, kind Op) merger {
	return merger{holder: holder, parent: parent, kind: kind, gate: parentGate{attr: parent, assigned: len(holder) > 0}}
}

// RFC 7644 Section 3.5.2.3: sub-attributes that are not specified in the "value" parameter are left unchanged.
func (m merger) merge(values map[string]any) error {
	keys := newKeys(m.holder)
	for name, incoming := range values {
		key := keys.resolve(name)
		if err := m.field(key, name, incoming); err != nil {
			return err
		}
		keys.add(key)
	}
	return nil
}

func (m merger) field(key, name string, incoming any) error {
	sub := subAttr(m.parent, name)
	v := shaped(incoming, sub.MultiValued)
	if existing, ok := m.holder[key].([]any); m.kind == OpAdd && sub.MultiValued && ok {
		v = freshElements(sub, existing, v.([]any))
	}
	before, final := m.holder[key], appended(m.holder[key], v, m.kind)
	if err := gateImmutableWrite(sub, before, final); err != nil {
		return err
	}
	if err := m.gate.check(sub, before, final); err != nil {
		return err
	}
	m.holder[key] = final
	return nil
}

func (g parentGate) check(sub *core.Attribute, before, final any) error {
	if g.attr == nil || g.attr.MultiValued || g.attr.Mutability != core.MutabilityImmutable || !g.assigned || value.Equal(sub, before, final) {
		return nil
	}
	return scimerrors.ErrMutability(strconv.Quote(g.attr.Name) + " is immutable")
}

// RFC 7643 Section 7: an assigned immutable sub-attribute rejects any write that would change it.
func gateImmutableWrite(sub *core.Attribute, before, candidate any) error {
	if sub.Mutability != core.MutabilityImmutable || value.IsUnassigned(before) || value.Equal(sub, before, candidate) {
		return nil
	}
	return scimerrors.ErrMutability(strconv.Quote(sub.Name) + " is immutable")
}

// RFC 7644 3.5.2.1 - a value written to a multi-valued attribute is an array; RFC 7643 Section 2.5 treats "null" the same as an empty array.
func shaped(value any, multiValued bool) any {
	if !multiValued {
		return value
	}
	if value == nil {
		return []any{}
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
