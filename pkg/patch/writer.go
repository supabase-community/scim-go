package patch

import (
	"strconv"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type merger struct {
	holder core.Object
	parent *core.Attribute
	kind   Op
	gate   parentGate
}

// RFC 7643 Section 7: an immutable complex attribute SHALL NOT be updated, including through its sub-attributes.
type parentGate struct {
	attr *core.Attribute
}

func newMerger(holder core.Object, parent *core.Attribute, kind Op) merger {
	return merger{holder: holder, parent: parent, kind: kind, gate: newParentGate(parent, holder)}
}

func newParentGate(attr *core.Attribute, holder core.Object) parentGate {
	if attr == nil || attr.MultiValued || attr.Mutability != core.MutabilityImmutable || len(holder) == 0 {
		return parentGate{}
	}
	return parentGate{attr: attr}
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
		v = newIndex(sub, existing).fresh(v.([]any))
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
	if g.attr == nil || value.Equal(sub, before, final) {
		return nil
	}
	return errImmutable(g.attr)
}

// RFC 7643 Section 7: an assigned immutable sub-attribute rejects any write that would change it.
func gateImmutableWrite(sub *core.Attribute, before, candidate any) error {
	if sub.Mutability != core.MutabilityImmutable || value.IsUnassigned(before) || value.Equal(sub, before, candidate) {
		return nil
	}
	return errImmutable(sub)
}

func errImmutable(attr *core.Attribute) error {
	return scimerrors.ErrMutability(strconv.Quote(attr.Name) + " is immutable")
}

// RFC 7643 Section 2.5: the null value or an empty array SHALL be considered equivalent for a multi-valued attribute.
func shaped(incoming any, multiValued bool) any {
	if !multiValued {
		return incoming
	}
	if incoming == nil {
		return []any{}
	}
	if list, ok := incoming.([]any); ok {
		return list
	}
	return []any{incoming}
}

func set(o core.Object, name string, incoming any, kind Op) {
	o.Set(name, appended(o.Get(name), incoming, kind))
}

func appended(before, incoming any, kind Op) any {
	if list, ok := before.([]any); kind == OpAdd && ok {
		return append(list, shaped(incoming, true).([]any)...)
	}
	return incoming
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
