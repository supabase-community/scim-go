package patch

import (
	"encoding/json"
	"strconv"

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
	filter    filterState
	budget    *budget
	indexes   indexCache
}

func (t *target) write(kind Op, incoming any) error {
	replace := t.replacer()
	if replace.selectsValues(incoming) {
		return replace.replace(kind, incoming)
	}
	if t.key() == "" && !isObject(incoming) {
		return scimerrors.ErrInvalidValue(`"value" must be an object when "path" has no sub-attribute`)
	}
	stripReadOnly(t.owner(), incoming)
	holders, err := t.holders(true)
	if err != nil {
		return err
	}
	if err := t.chargeOutput(len(holders), incoming); err != nil {
		return err
	}
	adds := t.listAdd()
	incoming = adds.fresh(kind, incoming)
	before := t.elements()
	written := t.filter.written(before, kind, t.path.SubAttribute)
	for _, holder := range holders {
		if err := t.put(holder, kind, value.Clone(incoming)); err != nil {
			return err
		}
	}
	adds.demoteIfPromoted(kind, before, written, incoming)
	adds.record(kind, incoming)
	t.unassignEmptyExtension()
	return nil
}

func (t *target) chargeOutput(count int, incoming any) error {
	encoded, err := json.Marshal(incoming)
	if err != nil {
		return scimerrors.ErrInternal("could not encode the request body")
	}
	return t.budget.chargeBytes(count * len(encoded))
}

func (t *target) put(holder core.Object, kind Op, incoming any) error {
	values := value.AsObject(incoming)
	isObject := values != nil
	switch {
	case t.key() == "":
		return newMerger(holder, t.parent, kind).merge(values)
	case isObject && t.attr.Type == core.TypeComplex && !t.attr.MultiValued:
		nested, err := child(holder, t.key())
		if err != nil {
			return err
		}
		return newMerger(nested, t.attr, kind).merge(values)
	}
	shapedValue := t.listAdd().dedupedAdd(holder, kind, shaped(incoming, t.attr.MultiValued))
	if err := t.overwritable(holder, kind, shapedValue); err != nil {
		return err
	}
	set(holder, t.key(), carryForward(t.attr, kind, t.elements(), shapedValue), kind)
	return nil
}

// RFC 7644 Section 3.5.2: an operation that is not compatible with an attribute's mutability SHALL return an error.
func (t *target) overwritable(holder core.Object, kind Op, candidate any) error {
	before := holder.Get(t.key())
	final := appended(before, candidate, kind)
	if err := gateImmutableWrite(t.attr, before, final); err != nil {
		return err
	}
	return newParentGate(t.subParent(), holder).check(t.attr, before, final)
}

// subParent reports the singular complex parent a sub-attribute write also changes, per RFC 7643 Section 7; nil for a plain top-level path.
func (t *target) subParent() *core.Attribute {
	if t.path.SubAttribute == "" {
		return nil
	}
	return t.parent
}

func (t *target) unassignEmptyExtension() {
	if container, _ := t.container(false); t.extension != "" && value.AllUnassigned(container) {
		t.root.Remove(t.extension)
	}
}

func (t *target) holders(create bool) ([]core.Object, error) {
	container, err := t.container(create)
	if err != nil {
		return nil, err
	}
	switch {
	case t.filter.match != nil:
		return t.filter.holders(container, t.path.Name)
	case t.path.SubAttribute == "":
		return []core.Object{container}, nil
	case create && t.parent.MultiValued:
		return nil, scimerrors.ErrInvalidPath(`"path" targets a non-complex attribute`)
	case create:
		nested, err := child(container, t.path.Name)
		return []core.Object{nested}, err
	}
	return t.typedHolders(container)
}

func (t *target) typedHolders(container core.Object) ([]core.Object, error) {
	held := container.Get(t.path.Name)
	if object := value.AsObject(held); object != nil {
		return []core.Object{object}, nil
	}
	if list, ok := held.([]any); ok {
		return members(list, nil)
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
	return value.AsObject(t.root.Get(t.extension)), nil
}

func (t *target) elements() []any {
	container, _ := t.container(false)
	elements, _ := container.Get(t.path.Name).([]any)
	return elements
}

func (t *target) replacer() replacer {
	return replacer{t: t}
}

func (t *target) listAdd() listAdd {
	return listAdd{t: t}
}

func (t *target) remover() remover {
	return remover{t: t}
}

func (t *target) owner() *core.Attribute {
	if t.key() == "" {
		return t.parent
	}
	return t.attr
}

func (t *target) key() string {
	if t.filter.match != nil || t.path.SubAttribute != "" {
		return t.path.SubAttribute
	}
	return t.path.Name
}

func isObject(candidate any) bool {
	return value.AsObject(candidate) != nil
}

// RFC 7644 Section 3.5.1: readOnly values provided SHALL be ignored.
func stripReadOnly(attr *core.Attribute, incoming any) {
	if list, ok := incoming.([]any); ok {
		for _, element := range list {
			stripReadOnly(attr, element)
		}
		return
	}
	object := value.AsObject(incoming)
	for name := range object {
		if subAttr(attr, name).Mutability == core.MutabilityReadOnly {
			delete(object, name)
		}
	}
}

// RFC 7644 Section 3.5.2.2: an attribute that is removed or becomes unassigned and is required SHALL return "mutability".
func gateRequired(attr *core.Attribute, becomesUnassigned bool) error {
	if !attr.Required || !becomesUnassigned {
		return nil
	}
	return scimerrors.ErrMutability(strconv.Quote(attr.Name) + " is required")
}

func subAttr(parent *core.Attribute, name string) *core.Attribute {
	if sub := parent.SubAttribute(name); sub != nil {
		return sub
	}
	return permissiveAttr
}
