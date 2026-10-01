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

// RFC 7644 Section 3.5.2: the "valuePath" rule allows specific values of a complex multi-valued attribute to be selected.
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
	before := t.elements()
	written := t.written(before, kind)
	for _, holder := range holders {
		if err := t.put(holder, kind, clone(value)); err != nil {
			return err
		}
	}
	t.demoteIfPromoted(kind, before, written, value)
	t.record(kind, value)
	return nil
}

func (t *target) demoteIfPromoted(kind Op, before []any, written func(int) bool, value any) {
	switch {
	case !promotes(t.key(), value):
		return
	case t.isListAdd(kind):
		t.demotePrimaries(before, value)
	default:
		demote(t.elements(), written)
	}
}

// RFC 7644 Section 3.5.2: the hot path for repeated top-level list-adds of "primary" - demotes only the
// elements this attribute's cached index already knows hold it, instead of rescanning the whole list.
func (t *target) demotePrimaries(before []any, added any) {
	ix := t.index(before)
	for _, held := range ix.primaries {
		setPrimaryFalse(held)
	}
	after := t.elements()
	ix.primaries = ix.primaries[:0]
	for _, element := range after[len(before):] {
		if value.Primary(element) {
			ix.primaries = append(ix.primaries, element)
		}
	}
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
		return newMerger(holder, t.parent, kind).merge(values)
	case isObject && t.attr.Type == core.TypeComplex && !t.attr.MultiValued:
		nested, err := child(holder, t.key())
		if err != nil {
			return err
		}
		return newMerger(nested, t.attr, kind).merge(values)
	}
	shapedValue := t.dedupedAdd(holder, kind, shaped(value, t.attr.MultiValued))
	if err := t.overwritable(holder, kind, shapedValue); err != nil {
		return err
	}
	set(holder, t.key(), carryImmutable(t.attr, kind, t.elements(), shapedValue), kind)
	return nil
}

// RFC 7644 Section 3.5.2.1: if the target location already contains the value specified, no changes SHOULD be made.
func (t *target) dedupedAdd(holder core.Object, kind Op, shapedValue any) any {
	existing, ok := holder.Get(t.key()).([]any)
	if kind != OpAdd || !t.attr.MultiValued || !ok || t.isListAdd(kind) {
		return shapedValue
	}
	return newIndex(t.attr, existing).fresh(shapedValue.([]any))
}

// RFC 7644 Section 3.5.2: an operation that is not compatible with an attribute's mutability SHALL return an error.
func (t *target) overwritable(holder core.Object, kind Op, candidate any) error {
	before := holder.Get(t.key())
	final := appended(before, candidate, kind)
	if err := gateImmutableWrite(t.attr, before, final); err != nil {
		return err
	}
	gate := newParentGate(t.subParent(), holder)
	if err := gate.check(t.attr, before, final); err != nil {
		return err
	}
	if _, appends := before.([]any); appends && kind == OpAdd {
		return nil
	}
	return eachSub(t.attr, before, gateRemove)
}

// subParent reports the singular complex parent a sub-attribute write also changes, per RFC 7643 Section 7; nil for a plain top-level path.
func (t *target) subParent() *core.Attribute {
	if t.path.SubAttribute == "" {
		return nil
	}
	return t.parent
}

func (t *target) remove() error {
	clear(t.indexes)
	if t.filter.match != nil && t.path.SubAttribute == "" {
		return t.drop()
	}
	holders, err := t.holders(false)
	if err != nil {
		return err
	}
	for _, holder := range holders {
		if err := t.gateHolderRemoval(holder); err != nil {
			return err
		}
	}
	for _, holder := range holders {
		holder.Remove(t.key())
	}
	t.unassignEmptyExtension()
	return nil
}

// RFC 7643 Section 7: an assigned immutable attribute SHALL NOT be updated, including by removal.
func (t *target) gateHolderRemoval(holder core.Object) error {
	held := holder.Get(t.key())
	if err := eachSub(t.attr, held, gateRemove); err != nil {
		return err
	}
	if err := gateRequired(t.attr, held); err != nil {
		return err
	}
	if err := gateImmutableWrite(t.attr, held, nil); err != nil {
		return err
	}
	return newParentGate(t.subParent(), holder).check(t.attr, held, nil)
}

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

func (t *target) matchedHolders(container core.Object) ([]core.Object, error) {
	elements, _ := container.Get(t.path.Name).([]any)
	matched, err := t.matches(elements)
	if err != nil {
		return nil, err
	}
	t.filter.matched = matched
	return members(elements, matched)
}

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

func (t *target) fresh(candidate any) []any {
	return t.index(t.elements()).fresh(shaped(candidate, true).([]any))
}

func (t *target) record(kind Op, added any) {
	if !t.isListAdd(kind) || promotes(t.key(), added) && !keyedByValue(t.attr) {
		clear(t.indexes)
		return
	}
	elements := t.elements()
	stored := len(elements) - len(added.([]any))
	t.index(elements[:stored]).add(elements[stored:])
}

func (t *target) index(elements []any) *index {
	return t.indexes.lookup(t.extension, t.path.Name, t.attr, elements)
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
		return eachSubField(attr, v, visit)
	case []any:
		return eachSubElement(attr, v, visit)
	}
	return nil
}

func eachSubField(attr *core.Attribute, fields map[string]any, visit func(sub *core.Attribute, held any) error) error {
	for name, held := range fields {
		if err := visit(subAttr(attr, name), held); err != nil {
			return err
		}
	}
	return nil
}

func eachSubElement(attr *core.Attribute, elements []any, visit func(sub *core.Attribute, held any) error) error {
	for _, element := range elements {
		if err := eachSub(attr, element, visit); err != nil {
			return err
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

// RFC 7644 Section 3.5.2.2: an attribute that is removed or becomes unassigned and is required SHALL return "mutability".
func gateRequired(attr *core.Attribute, held any) error {
	if !attr.Required || value.IsUnassigned(held) {
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

func clone(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		cloned := make(map[string]any, len(typed))
		for key, element := range typed {
			cloned[key] = clone(element)
		}
		return cloned
	case []any:
		cloned := make([]any, len(typed))
		for i, element := range typed {
			cloned[i] = clone(element)
		}
		return cloned
	default:
		return typed
	}
}
