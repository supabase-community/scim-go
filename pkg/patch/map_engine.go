package patch

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"strconv"
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type mapEngine struct {
	catalog catalog
}

func newMapEngine(schemas []*core.Schema) *mapEngine {
	return &mapEngine{
		catalog: catalog{schemas: schemas},
	}
}

func (r *mapEngine) apply(resource map[string]any, ops []Operation) error {
	working := r.cloneMap(resource)
	for _, op := range ops {
		if err := r.applyOp(working, op); err != nil {
			return err
		}
	}
	for key := range resource {
		delete(resource, key)
	}
	maps.Copy(resource, working)
	return nil
}

func (r *mapEngine) applyOp(resource map[string]any, op Operation) error {
	switch Op(strings.ToLower(string(op.Op))) {
	case OpAdd:
		return r.applyWrite(resource, op, OpAdd)
	case OpReplace:
		return r.applyWrite(resource, op, OpReplace)
	case OpRemove:
		return r.applyRemove(resource, op)
	default:
		return scimerrors.ErrInvalidSyntax(`"op" must be "add", "remove", or "replace"`)
	}
}

func (r *mapEngine) applyWrite(resource map[string]any, op Operation, kind Op) error {
	if op.Path == "" {
		return r.applyMerge(resource, op.Value, kind)
	}

	path, err := filter.NewPath(op.Path)
	if err != nil {
		return scimerrors.ErrInvalidPath(err.Error())
	}
	if len(op.Value) == 0 {
		return scimerrors.ErrInvalidValue(`"value" is required for "add" and "replace"`)
	}
	value, err := r.decodeValue(op.Value)
	if err != nil {
		return err
	}

	w := valueWrite{path: path, value: value, appendMode: kind == OpAdd}
	if path.ValueFilter != nil {
		return r.applyValueWrite(resource, w)
	}
	attr, err := r.guard(path, resource)
	if err != nil {
		return r.skipOrFail(err)
	}
	return r.writePath(resource, w, attr)
}

func (r *mapEngine) applyRemove(resource map[string]any, op Operation) error {
	if op.Path == "" {
		return scimerrors.ErrNoTarget(`"remove" requires a "path"`)
	}
	path, err := filter.NewPath(op.Path)
	if err != nil {
		return scimerrors.ErrInvalidPath(err.Error())
	}
	if path.ValueFilter != nil {
		return r.applyValueRemove(resource, path)
	}
	if _, err := r.guard(path, resource); err != nil {
		return r.skipOrFail(err)
	}
	r.removePath(resource, path)
	return nil
}

func (r *mapEngine) removePath(resource map[string]any, path filter.Path) {
	if path.SubAttribute == "" {
		delete(resource, object(resource).key(path.Name))
		return
	}
	key := object(resource).key(path.Name)
	if nested, ok := resource[key].(map[string]any); ok {
		delete(nested, object(nested).key(path.SubAttribute))
	}
}

func (r *mapEngine) cloneMap(source map[string]any) map[string]any {
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = r.cloneValue(value)
	}
	return cloned
}

func (r *mapEngine) cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return r.cloneMap(typed)
	case []any:
		cloned := make([]any, len(typed))
		for i, element := range typed {
			cloned[i] = r.cloneValue(element)
		}
		return cloned
	default:
		return typed
	}
}

func (r *mapEngine) applyMerge(resource map[string]any, raw json.RawMessage, kind Op) error {
	values, err := r.decodeMap(raw)
	if err != nil {
		return scimerrors.ErrInvalidValue(`"value" must be an object when "path" is omitted`)
	}
	appendMode := kind == OpAdd
	for key, value := range values {
		attr, err := r.guard(r.catalog.topLevelPath(key), resource)
		if err != nil {
			if errors.Is(err, errSkip) {
				continue
			}
			return err
		}
		r.setKey(resource, key, r.shape(value, attr.MultiValued), appendMode)
	}
	return nil
}

func (r *mapEngine) decodeMap(raw []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var doc map[string]any
	if err := decoder.Decode(&doc); err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, errors.New("scim: not a JSON object")
	}
	return doc, nil
}

func (r *mapEngine) decodeValue(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, scimerrors.ErrInvalidValue(`"value" is not valid JSON`)
	}
	return value, nil
}

func (r *mapEngine) applyValueWrite(resource map[string]any, w valueWrite) error {
	parent, err := r.guardParent(w.path, resource)
	if err != nil {
		return r.skipOrFail(err)
	}
	pred, err := matcher{catalog: r.catalog}.compile(w.path.ValueFilter, parent)
	if err != nil {
		return err
	}
	attr := parent
	if w.path.SubAttribute != "" {
		attr, err = r.catalog.attrFor(w.path)
		if err != nil {
			return err
		}
	}

	key := object(resource).key(w.path.Name)
	elements, _ := resource[key].([]any)
	matched := 0
	for _, element := range elements {
		member, ok := element.(map[string]any)
		if !ok || !pred(member) {
			continue
		}
		matched++
		if err := r.writeMember(member, w, attr); err != nil {
			if errors.Is(err, errSkip) {
				continue
			}
			return err
		}
	}
	if matched == 0 {
		return scimerrors.ErrNoTarget(`"path" filter matched no elements`)
	}
	resource[key] = elements
	return nil
}

func (r *mapEngine) writeMember(member map[string]any, w valueWrite, attr *core.Attribute) error {
	if w.path.SubAttribute != "" {
		if err := r.checkMutability(attr, object(member).has(w.path.SubAttribute)); err != nil {
			return err
		}
		r.setKey(member, w.path.SubAttribute, r.shape(w.value, attr.MultiValued), w.appendMode)
		return nil
	}
	values, ok := w.value.(map[string]any)
	if !ok {
		return scimerrors.ErrInvalidValue(`"value" must be an object when "path" has no sub-attribute`)
	}
	return r.mergeMember(member, values, w.appendMode, attr)
}

func (r *mapEngine) mergeMember(member, values map[string]any, appendMode bool, attr *core.Attribute) error {
	for key, value := range values {
		sub := r.catalog.subAttr(attr, key)
		if err := r.checkMutability(sub, object(member).has(key)); err != nil {
			if errors.Is(err, errSkip) {
				continue
			}
			return err
		}
		r.setKey(member, key, r.shape(value, sub.MultiValued), appendMode)
	}
	return nil
}

func (r *mapEngine) applyValueRemove(resource map[string]any, path filter.Path) error {
	parent, err := r.guardParent(path, resource)
	if err != nil {
		return r.skipOrFail(err)
	}
	pred, err := matcher{catalog: r.catalog}.compile(path.ValueFilter, parent)
	if err != nil {
		return err
	}
	if _, err := r.guard(path, resource); err != nil {
		return r.skipOrFail(err)
	}

	key := object(resource).key(path.Name)
	elements, _ := resource[key].([]any)
	if path.SubAttribute != "" {
		return r.removeMemberSub(elements, path.SubAttribute, pred)
	}
	kept, matched := r.partition(elements, pred)
	if matched == 0 {
		return scimerrors.ErrNoTarget(`"path" filter matched no elements`)
	}
	resource[key] = kept
	return nil
}

func (r *mapEngine) partition(elements []any, pred predicate) ([]any, int) {
	kept := make([]any, 0, len(elements))
	matched := 0
	for _, element := range elements {
		if member, ok := element.(map[string]any); ok && pred(member) {
			matched++
			continue
		}
		kept = append(kept, element)
	}
	return kept, matched
}

func (r *mapEngine) removeMemberSub(elements []any, sub string, pred predicate) error {
	matched := 0
	for _, element := range elements {
		if member, ok := element.(map[string]any); ok && pred(member) {
			delete(member, object(member).key(sub))
			matched++
		}
	}
	if matched == 0 {
		return scimerrors.ErrNoTarget(`"path" filter matched no elements`)
	}
	return nil
}

func (r *mapEngine) guard(path filter.Path, resource map[string]any) (*core.Attribute, error) {
	attr, err := r.catalog.attrFor(path)
	if err != nil {
		return nil, err
	}
	return attr, r.checkMutability(attr, r.attrExists(resource, path))
}

func (r *mapEngine) checkMutability(attr *core.Attribute, present bool) error {
	switch attr.Mutability {
	case core.MutabilityReadOnly:
		return errSkip
	case core.MutabilityImmutable:
		if present {
			return scimerrors.ErrMutability(strconv.Quote(attr.Name) + " is immutable")
		}
	}
	return nil
}

func (r *mapEngine) skipOrFail(err error) error {
	if errors.Is(err, errSkip) {
		return nil
	}
	return err
}

func (r *mapEngine) writePath(resource map[string]any, w valueWrite, attr *core.Attribute) error {
	value := r.shape(w.value, attr.MultiValued)
	if w.path.SubAttribute == "" {
		r.setKey(resource, w.path.Name, value, w.appendMode)
		return nil
	}
	key := object(resource).key(w.path.Name)
	nested, ok := resource[key].(map[string]any)
	if !ok {
		if resource[key] != nil {
			return scimerrors.ErrInvalidPath(`"path" targets a non-complex attribute`)
		}
		nested = map[string]any{}
	}
	r.setKey(nested, w.path.SubAttribute, value, w.appendMode)
	resource[key] = nested
	return nil
}

func (r *mapEngine) guardParent(path filter.Path, resource map[string]any) (*core.Attribute, error) {
	parent, err := r.catalog.parentAttr(path)
	if err != nil {
		return parent, err
	}
	base := path
	base.SubAttribute = ""
	return parent, r.checkMutability(parent, r.attrExists(resource, base))
}

func (r *mapEngine) attrExists(resource map[string]any, path filter.Path) bool {
	if path.SubAttribute == "" {
		return object(resource).has(path.Name)
	}
	if nested, ok := resource[object(resource).key(path.Name)].(map[string]any); ok {
		return object(nested).has(path.SubAttribute)
	}
	return false
}

func (r *mapEngine) setKey(container map[string]any, key string, value any, appendMode bool) {
	existing := object(container).key(key)
	if appendMode {
		if before, ok := container[existing].([]any); ok {
			container[existing] = append(before, r.members(value)...)
			return
		}
	}
	container[existing] = value
}

// RFC 7644 3.5.2.1 - a value written to a multi-valued attribute is an array.
func (r *mapEngine) shape(value any, multiValued bool) any {
	if !multiValued {
		return value
	}
	return r.members(value)
}

func (r *mapEngine) members(value any) []any {
	if list, ok := value.([]any); ok {
		return list
	}
	return []any{value}
}

