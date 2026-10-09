package patch

import (
	"encoding/json"
	"errors"
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

func (t *target) write(kind Op, incoming any) error {
	if t.selectsValues(incoming) {
		return t.replaceValues(kind, incoming)
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
	incoming = t.fresh(kind, incoming)
	before := t.elements()
	written := t.written(before, kind)
	for _, holder := range holders {
		if err := t.put(holder, kind, value.Clone(incoming)); err != nil {
			return err
		}
	}
	t.demoteIfPromoted(kind, before, written, incoming)
	t.record(kind, incoming)
	t.unassignEmptyExtension()
	return nil
}

func (t *target) selectsValues(incoming any) bool {
	switch {
	case t.filter.match == nil || t.path.SubAttribute != "":
		return false
	case t.parent != permissiveAttr:
		return isSimple(t.parent)
	}
	elements := t.elements()
	return len(elements) > 0 && !isObject(elements[0]) && !isObject(incoming)
}

// RFC 7644 Section 3.5.2.3: all matching record values SHALL be replaced.
func (t *target) replaceValues(kind Op, incoming any) error {
	if kind != OpReplace {
		return scimerrors.ErrInvalidPath(`"add" cannot target values selected by a filter`)
	}
	if _, ok := t.typeOf(incoming).Coerce(incoming); !ok || incoming == nil {
		return scimerrors.ErrInvalidValue(`"value" must be a single value of the attribute's type`)
	}
	elements := t.elements()
	matched, err := t.matches(elements)
	if err != nil {
		return err
	}
	count := tally(matched)
	if count == 0 {
		return scimerrors.ErrNoTarget(`"path" matched no elements`)
	}
	if err := t.chargeOutput(1, incoming); err != nil {
		return err
	}
	return t.rewrite(elements, matched, incoming)
}

func (t *target) typeOf(incoming any) *core.Attribute {
	if t.attr == permissiveAttr {
		return inferred(incoming)
	}
	return t.attr
}

func (t *target) rewrite(elements []any, matched []bool, incoming any) error {
	after := t.substitute(elements, matched, incoming)
	if err := gateImmutableWrite(t.attr, elements, after); err != nil {
		return err
	}
	container, _ := t.container(false)
	container.Set(t.path.Name, after)
	clear(t.indexes)
	return nil
}

func (t *target) substitute(elements []any, matched []bool, incoming any) []any {
	after := elements[:0]
	if t.attr.Mutability == core.MutabilityImmutable {
		after = make([]any, 0, len(elements))
	}
	placed := incoming == nil || t.keeps(elements, matched, incoming)
	for i, element := range elements {
		switch {
		case !matched[i]:
			after = append(after, element)
		case !placed:
			after = append(after, incoming)
			placed = true
		}
	}
	return after
}

func (t *target) keeps(elements []any, matched []bool, incoming any) bool {
	for i, element := range elements {
		if !matched[i] && value.Equal(t.attr, element, incoming) {
			return true
		}
	}
	return false
}

func (t *target) demoteIfPromoted(kind Op, before []any, written func(int) bool, incoming any) {
	switch {
	case !promotes(t.key(), incoming):
		return
	case t.isListAdd(kind):
		t.demotePrimaries(before)
	default:
		demote(t.elements(), written)
	}
}

func (t *target) demotePrimaries(before []any) {
	ix := t.index(before)
	for _, held := range ix.primaries {
		setPrimary(held, false)
	}
	after := t.elements()
	ix.primaries = ix.primaries[:0]
	for _, element := range after[len(before):] {
		if value.Primary(element) {
			ix.primaries = append(ix.primaries, element)
		}
	}
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
	shapedValue := t.dedupedAdd(holder, kind, shaped(incoming, t.attr.MultiValued))
	if err := t.overwritable(holder, kind, shapedValue); err != nil {
		return err
	}
	set(holder, t.key(), carryForward(t.attr, kind, t.elements(), shapedValue), kind)
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
	return newParentGate(t.subParent(), holder).check(t.attr, before, final)
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
	err := t.removeValues()
	if t.filter.match != nil && errors.Is(err, scimerrors.ErrNoTarget("")) {
		return nil // RFC 7644 Section 3.5.2.2: if nothing matched, no changes should be made and a success response should be returned.
	}
	return err
}

func (t *target) removeValues() error {
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
	if err := gateRequired(t.attr, !value.IsUnassigned(held)); err != nil {
		return err
	}
	if err := gateImmutableWrite(t.attr, held, nil); err != nil {
		return err
	}
	return newParentGate(t.subParent(), holder).check(t.attr, held, nil)
}

func (t *target) unassignEmptyExtension() {
	if container, _ := t.container(false); t.extension != "" && value.AllUnassigned(container) {
		t.root.Remove(t.extension)
	}
}

func (t *target) drop() error {
	elements := t.elements()
	matched, err := t.matches(elements)
	if err != nil {
		return err
	}
	count := tally(matched)
	if count == 0 {
		return scimerrors.ErrNoTarget(`"path" matched no elements`)
	}
	if err := gateRequired(t.attr, count == len(elements)); err != nil {
		return err
	}
	return t.rewrite(elements, matched, nil)
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
	held := container.Get(t.path.Name)
	if object := value.AsObject(held); object != nil {
		return []core.Object{object}, nil
	}
	if list, ok := held.([]any); ok {
		return members(list, nil)
	}
	return nil, nil
}

func (t *target) matches(elements []any) ([]bool, error) {
	if err := t.budget.charge(len(elements) * t.filter.clauses); err != nil {
		return nil, err
	}
	matched := make([]bool, len(elements))
	for i, element := range elements {
		matched[i] = (isObject(element) || t.parent.Type != core.TypeComplex) && t.filter.match(element)
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
	return value.AsObject(t.root.Get(t.extension)), nil
}

func (t *target) elements() []any {
	container, _ := t.container(false)
	elements, _ := container.Get(t.path.Name).([]any)
	return elements
}

func (t *target) isListAdd(kind Op) bool {
	return kind == OpAdd && t.filter.match == nil && t.path.Name != "" && t.path.SubAttribute == "" && t.attr.MultiValued
}

func (t *target) fresh(kind Op, candidate any) any {
	if !t.isListAdd(kind) {
		return candidate
	}
	return t.index(t.elements()).fresh(shaped(candidate, true).([]any))
}

func (t *target) record(kind Op, added any) {
	if !t.isListAdd(kind) || promotes(t.key(), added) && !value.KeyedByIdentity(t.attr) {
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

func isObject(candidate any) bool {
	return value.AsObject(candidate) != nil
}

func tally(matched []bool) int {
	count := 0
	for _, hit := range matched {
		if hit {
			count++
		}
	}
	return count
}

func members(elements []any, matched []bool) ([]core.Object, error) {
	holders := []core.Object{}
	for i, element := range elements {
		if member := value.AsObject(element); member != nil && (matched == nil || matched[i]) {
			holders = append(holders, member)
		}
	}
	if len(holders) == 0 {
		return nil, scimerrors.ErrNoTarget(`"path" matched no elements`)
	}
	return holders, nil
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
