package patch

import (
	"encoding/json"
	"slices"

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
}

// filterState holds the compiled value-filter predicate and its match results; RFC 7644, Section 3.5.2: a "path" value filter targets specific elements of a multi-valued attribute.
type filterState struct {
	match   predicate
	clauses int
	matched []bool
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
	if err := t.chargeOutput(holders, value); err != nil {
		return err
	}
	if t.isListAdd(kind) {
		value = t.fresh(value)
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

func (t *target) chargeOutput(holders []core.Object, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return scimerrors.ErrInternal("could not encode the request body")
	}
	return t.budget.chargeBytes(len(holders) * len(encoded))
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
	shapedValue := shaped(value, t.attr.MultiValued)
	if err := t.overwritable(holder, kind, shapedValue); err != nil {
		return err
	}
	set(holder, t.key(), carryImmutable(t.attr, kind, t.elements(), shapedValue), kind)
	return nil
}

// RFC 7644 Section 3.5.2.2: a value that becomes unassigned and is read-only SHALL return "mutability".
func (t *target) overwritable(holder core.Object, kind Op, candidate any) error {
	before := holder.Get(t.key())
	if _, appends := before.([]any); appends && kind == OpAdd {
		return nil
	}
	if err := gateImmutableWrite(t.attr, before, candidate); err != nil {
		return err
	}
	return eachSub(t.attr, before, gateRemove)
}

func (t *target) remove() error {
	if t.filter.match != nil && t.path.SubAttribute == "" {
		return t.drop()
	}
	holders, err := t.holders(false)
	if err != nil {
		return err
	}
	for _, holder := range holders {
		held := holder.Get(t.key())
		if err := eachSub(t.attr, held, gateRemove); err != nil {
			return err
		}
		// RFC 7643 Section 7: an assigned immutable sub-attribute cannot be removed while its element survives.
		if t.path.SubAttribute != "" {
			if err := gateImmutableWrite(t.attr, held, nil); err != nil {
				return err
			}
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
		return nil // RFC 7644 Section 3.5.2.2: a filter matching no value makes no change and still succeeds.
	}
	container.Set(t.path.Name, kept)
	return nil
}

func (t *target) holders(create bool) ([]core.Object, error) {
	container, err := t.container(create)
	if err != nil {
		return nil, err
	}
	switch {
	case t.filter.match != nil:
		return t.matchedHolders(container)
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

// matchedHolders resolves a value-filtered path to the elements the filter selects, per RFC 7644, Section 3.5.2.
func (t *target) matchedHolders(container core.Object) ([]core.Object, error) {
	elements, _ := container.Get(t.path.Name).([]any)
	matched, err := t.matches(elements)
	if err != nil {
		return nil, err
	}
	t.filter.matched = matched
	return members(elements, matched)
}

// typedHolders resolves a plain sub-attribute path by the runtime shape of its container value.
func (t *target) typedHolders(container core.Object) ([]core.Object, error) {
	switch value := container.Get(t.path.Name).(type) {
	case map[string]any:
		return []core.Object{value}, nil
	case []any:
		return members(value, nil)
	}
	return nil, nil
}

func (t *target) matches(elements []any) ([]bool, error) {
	if err := t.budget.charge(len(elements) * t.filter.clauses); err != nil {
		return nil, err
	}
	matched := make([]bool, len(elements))
	for i, element := range elements {
		member, ok := element.(map[string]any)
		matched[i] = ok && t.filter.match(member)
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

func (t *target) isListAdd(kind Op) bool {
	return kind == OpAdd && t.filter.match == nil && t.path.Name != "" && t.path.SubAttribute == "" && t.attr.MultiValued
}

// fresh drops elements already present in the target list, per RFC 7644, Section 3.5.2.1: "If the target location already contains the value specified, no changes SHOULD be made". Elements identified by a "value" sub-attribute are matched by that identity, per RFC 7643 Section 2.4.
func (t *target) fresh(candidate any) []any {
	elements := slices.Clone(shaped(candidate, true).([]any))
	if t.attr.SubAttribute("value") == nil {
		stored := value.NewSet(t.attr, t.elements())
		return slices.DeleteFunc(elements, stored.Contains)
	}
	stored := value.ByIdentity(t.attr, t.elements())
	return slices.DeleteFunc(elements, func(addition any) bool {
		_, exists := stored[value.Identity(t.attr, asMember(addition))]
		return exists
	})
}

func (t *target) written(elements []any, kind Op) func(int) bool {
	switch {
	case t.filter.match != nil:
		return func(i int) bool { return t.filter.matched[i] }
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
	if t.filter.match != nil || t.path.SubAttribute != "" {
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
