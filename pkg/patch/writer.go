package patch

import (
	"strconv"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// merger holds the state one merge shares across its fields, per RFC 7644 Section 3.5.2.3.
type merger struct {
	holder  core.Object
	parent  *core.Attribute
	kind    Op
	gate    parentGate
	indexes indexes
}

// parentGate reports whether a sub-attribute write also changes its immutable parent complex attribute, per RFC 7643 Section 7.
type parentGate struct {
	attr     *core.Attribute
	assigned bool
}

func newMerger(holder core.Object, parent *core.Attribute, kind Op, indexes indexes) merger {
	return merger{holder: holder, parent: parent, kind: kind, gate: newParentGate(parent, holder), indexes: indexes}
}

func newParentGate(attr *core.Attribute, holder core.Object) parentGate {
	return parentGate{attr: attr, assigned: len(holder) > 0}
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
	if _, ok := m.holder[key].([]any); m.kind == OpAdd && sub.MultiValued && ok {
		v = m.indexes.fresh(m.holder, key, sub, v.([]any))
	}
	before, final := m.holder[key], appended(m.holder[key], v, m.kind)
	if err := gateImmutableWrite(sub, before, final); err != nil {
		return err
	}
	if err := m.gate.check(sub, before, final); err != nil {
		return err
	}
	if m.kind != OpAdd {
		m.indexes.invalidate(m.holder, key)
	}
	m.holder[key] = final
	return nil
}

func (g parentGate) check(sub *core.Attribute, before, final any) error {
	if g.attr == nil || g.attr.MultiValued {
		return nil
	}
	return gateUnlessUnchanged(g.attr, g.assigned, value.Equal(sub, before, final))
}

// RFC 7643 Section 7: an assigned immutable sub-attribute rejects any write that would change it.
func gateImmutableWrite(sub *core.Attribute, before, candidate any) error {
	return gateUnlessUnchanged(sub, !value.IsUnassigned(before), value.Equal(sub, before, candidate))
}

// gateUnlessUnchanged rejects a write against an assigned immutable attr unless it leaves its value unchanged, per RFC 7643 Section 7.
func gateUnlessUnchanged(attr *core.Attribute, assigned, unchanged bool) error {
	if attr.Mutability != core.MutabilityImmutable || !assigned || unchanged {
		return nil
	}
	return scimerrors.ErrMutability(strconv.Quote(attr.Name) + " is immutable")
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
