package patch

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type document struct {
	catalog  catalog
	resource map[string]any
}

func newDocument(schemas []*core.Schema, resource map[string]any) *document {
	return &document{
		catalog:  catalog{schemas: schemas},
		resource: resource,
	}
}

func (r *document) applyAll(ops []Operation) error {
	working := r.cloneMap(r.resource)
	for _, op := range ops {
		if err := r.applyOp(working, op); err != nil {
			return err
		}
	}
	for key := range r.resource {
		delete(r.resource, key)
	}
	maps.Copy(r.resource, working)
	return nil
}

func (r *document) applyOp(resource map[string]any, op Operation) error {
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

func (r *document) applyWrite(resource map[string]any, op Operation, kind Op) error {
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
		return guard{}.skipOrFail(err)
	}
	return r.writePath(resource, w, attr)
}

func (r *document) applyRemove(resource map[string]any, op Operation) error {
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
		return guard{}.skipOrFail(err)
	}
	r.removePath(resource, path)
	return nil
}

func (r *document) removePath(resource map[string]any, path filter.Path) {
	if path.SubAttribute == "" {
		object(resource).remove(path.Name)
		return
	}
	key := object(resource).key(path.Name)
	if nested, ok := resource[key].(map[string]any); ok {
		object(nested).remove(path.SubAttribute)
	}
}

func (r *document) cloneMap(source map[string]any) map[string]any {
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = r.cloneValue(value)
	}
	return cloned
}

func (r *document) cloneValue(value any) any {
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

func (r *document) applyMerge(resource map[string]any, raw json.RawMessage, kind Op) error {
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
		object(resource).set(key, r.shape(value, attr.MultiValued), appendMode)
	}
	return nil
}

func (r *document) decodeMap(raw []byte) (map[string]any, error) {
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

func (r *document) decodeValue(raw json.RawMessage) (any, error) {
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

func (r *document) applyValueWrite(resource map[string]any, w valueWrite) error {
	parent, err := r.guardParent(w.path, resource)
	if err != nil {
		return guard{}.skipOrFail(err)
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

func (r *document) writeMember(member map[string]any, w valueWrite, attr *core.Attribute) error {
	if w.path.SubAttribute != "" {
		if err := (guard{}).permits(attr, object(member).has(w.path.SubAttribute)); err != nil {
			return err
		}
		object(member).set(w.path.SubAttribute, r.shape(w.value, attr.MultiValued), w.appendMode)
		return nil
	}
	values, ok := w.value.(map[string]any)
	if !ok {
		return scimerrors.ErrInvalidValue(`"value" must be an object when "path" has no sub-attribute`)
	}
	return r.mergeMember(member, values, w.appendMode, attr)
}

func (r *document) mergeMember(member, values map[string]any, appendMode bool, attr *core.Attribute) error {
	for key, value := range values {
		sub := r.catalog.subAttr(attr, key)
		if err := (guard{}).permits(sub, object(member).has(key)); err != nil {
			if errors.Is(err, errSkip) {
				continue
			}
			return err
		}
		object(member).set(key, r.shape(value, sub.MultiValued), appendMode)
	}
	return nil
}

func (r *document) applyValueRemove(resource map[string]any, path filter.Path) error {
	parent, err := r.guardParent(path, resource)
	if err != nil {
		return guard{}.skipOrFail(err)
	}
	pred, err := matcher{catalog: r.catalog}.compile(path.ValueFilter, parent)
	if err != nil {
		return err
	}
	if _, err := r.guard(path, resource); err != nil {
		return guard{}.skipOrFail(err)
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

func (r *document) partition(elements []any, pred predicate) ([]any, int) {
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

func (r *document) removeMemberSub(elements []any, sub string, pred predicate) error {
	matched := 0
	for _, element := range elements {
		if member, ok := element.(map[string]any); ok && pred(member) {
			object(member).remove(sub)
			matched++
		}
	}
	if matched == 0 {
		return scimerrors.ErrNoTarget(`"path" filter matched no elements`)
	}
	return nil
}

func (r *document) guard(path filter.Path, resource map[string]any) (*core.Attribute, error) {
	attr, err := r.catalog.attrFor(path)
	if err != nil {
		return nil, err
	}
	return attr, guard{}.permits(attr, r.attrExists(resource, path))
}

func (r *document) writePath(resource map[string]any, w valueWrite, attr *core.Attribute) error {
	value := r.shape(w.value, attr.MultiValued)
	if w.path.SubAttribute == "" {
		object(resource).set(w.path.Name, value, w.appendMode)
		return nil
	}
	nested, err := object(resource).child(w.path.Name)
	if err != nil {
		return err
	}
	nested.set(w.path.SubAttribute, value, w.appendMode)
	return nil
}

func (r *document) guardParent(path filter.Path, resource map[string]any) (*core.Attribute, error) {
	parent, err := r.catalog.parentAttr(path)
	if err != nil {
		return parent, err
	}
	base := path
	base.SubAttribute = ""
	return parent, guard{}.permits(parent, r.attrExists(resource, base))
}

func (r *document) attrExists(resource map[string]any, path filter.Path) bool {
	if path.SubAttribute == "" {
		return object(resource).has(path.Name)
	}
	if nested, ok := resource[object(resource).key(path.Name)].(map[string]any); ok {
		return object(nested).has(path.SubAttribute)
	}
	return false
}

// RFC 7644 3.5.2.1 - a value written to a multi-valued attribute is an array.
func (r *document) shape(value any, multiValued bool) any {
	if !multiValued {
		return value
	}
	return r.members(value)
}

func (r *document) members(value any) []any {
	if list, ok := value.([]any); ok {
		return list
	}
	return []any{value}
}
