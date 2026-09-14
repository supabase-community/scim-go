package patch

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"strconv"
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

var errSkip = errors.New("scim: skip readOnly attribute")

var permissiveAttr = &core.Attribute{}

type engine struct {
	schemas []*core.Schema
}

func newEngine(schemas []*core.Schema) *engine {
	return &engine{
		schemas: schemas,
	}
}

func (r *engine) apply(resource any, ops []Operation) error {
	if doc, ok := resource.(map[string]any); ok {
		return r.applyToMap(doc, ops)
	}

	raw, err := json.Marshal(resource)
	if err != nil {
		return scimerrors.ErrInvalidSyntax("resource cannot be encoded")
	}
	doc, err := r.decodeMap(raw)
	if err != nil {
		return scimerrors.ErrInvalidSyntax("resource is not a JSON object")
	}
	if err := r.applyToMap(doc, ops); err != nil {
		return err
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return scimerrors.ErrInternal("could not encode the patched resource")
	}
	return r.writeBack(resource, out)
}

func (r *engine) applyToMap(doc map[string]any, ops []Operation) error {
	working := r.cloneMap(doc)
	for _, op := range ops {
		if err := r.applyOp(working, op); err != nil {
			return err
		}
	}
	for key := range doc {
		delete(doc, key)
	}
	maps.Copy(doc, working)
	return nil
}

func (r *engine) applyOp(doc map[string]any, op Operation) error {
	switch Op(strings.ToLower(string(op.Op))) {
	case OpAdd:
		return r.applyWrite(doc, op, OpAdd)
	case OpReplace:
		return r.applyWrite(doc, op, OpReplace)
	case OpRemove:
		return r.applyRemove(doc, op)
	default:
		return scimerrors.ErrInvalidSyntax(`"op" must be "add", "remove", or "replace"`)
	}
}

func (r *engine) applyWrite(doc map[string]any, op Operation, kind Op) error {
	if op.Path == "" {
		return r.applyMerge(doc, op.Value, kind)
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
		return r.applyValueWrite(doc, w)
	}
	attr, err := r.guard(path, doc)
	if err != nil {
		return r.skipOrFail(err)
	}
	return r.writePath(doc, w, attr)
}

func (r *engine) applyMerge(doc map[string]any, raw json.RawMessage, kind Op) error {
	values, err := r.decodeMap(raw)
	if err != nil {
		return scimerrors.ErrInvalidValue(`"value" must be an object when "path" is omitted`)
	}
	appendMode := kind == OpAdd
	for key, value := range values {
		attr, err := r.guard(r.topLevelPath(key), doc)
		if err != nil {
			if errors.Is(err, errSkip) {
				continue
			}
			return err
		}
		r.setKey(doc, key, r.shape(value, isMultiValued(attr)), appendMode)
	}
	return nil
}

func (r *engine) topLevelPath(name string) filter.Path {
	var path filter.Path
	path.Name = name
	return path
}

func (r *engine) applyRemove(doc map[string]any, op Operation) error {
	if op.Path == "" {
		return scimerrors.ErrNoTarget(`"remove" requires a "path"`)
	}
	path, err := filter.NewPath(op.Path)
	if err != nil {
		return scimerrors.ErrInvalidPath(err.Error())
	}
	if path.ValueFilter != nil {
		return r.applyValueRemove(doc, path)
	}
	if _, err := r.guard(path, doc); err != nil {
		return r.skipOrFail(err)
	}
	r.removePath(doc, path)
	return nil
}

func (r *engine) removePath(doc map[string]any, path filter.Path) {
	if path.SubAttribute == "" {
		delete(doc, object(doc).key(path.Name))
		return
	}
	key := object(doc).key(path.Name)
	if nested, ok := doc[key].(map[string]any); ok {
		delete(nested, object(nested).key(path.SubAttribute))
	}
}

func (r *engine) applyValueWrite(doc map[string]any, w valueWrite) error {
	parent, err := r.guardParent(w.path, doc)
	if err != nil {
		return r.skipOrFail(err)
	}
	pred, err := matcher{}.compile(w.path.ValueFilter, parent)
	if err != nil {
		return err
	}
	attr := parent
	if w.path.SubAttribute != "" {
		attr, err = r.attrFor(w.path)
		if err != nil {
			return err
		}
	}

	key := object(doc).key(w.path.Name)
	elements, _ := doc[key].([]any)
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
	doc[key] = elements
	return nil
}

func (r *engine) writeMember(member map[string]any, w valueWrite, attr *core.Attribute) error {
	if w.path.SubAttribute != "" {
		if err := r.checkMutability(attr, object(member).has(w.path.SubAttribute)); err != nil {
			return err
		}
		r.setKey(member, w.path.SubAttribute, r.shape(w.value, isMultiValued(attr)), w.appendMode)
		return nil
	}
	values, ok := w.value.(map[string]any)
	if !ok {
		return scimerrors.ErrInvalidValue(`"value" must be an object when "path" has no sub-attribute`)
	}
	return r.mergeMember(member, values, w.appendMode, attr)
}

func (r *engine) mergeMember(member, values map[string]any, appendMode bool, attr *core.Attribute) error {
	for key, value := range values {
		sub := subAttr(attr, key)
		if err := r.checkMutability(sub, object(member).has(key)); err != nil {
			if errors.Is(err, errSkip) {
				continue
			}
			return err
		}
		r.setKey(member, key, r.shape(value, isMultiValued(sub)), appendMode)
	}
	return nil
}

func (r *engine) applyValueRemove(doc map[string]any, path filter.Path) error {
	parent, err := r.guardParent(path, doc)
	if err != nil {
		return r.skipOrFail(err)
	}
	pred, err := matcher{}.compile(path.ValueFilter, parent)
	if err != nil {
		return err
	}
	if _, err := r.guard(path, doc); err != nil {
		return r.skipOrFail(err)
	}

	key := object(doc).key(path.Name)
	elements, _ := doc[key].([]any)
	if path.SubAttribute != "" {
		return r.removeMemberSub(elements, path.SubAttribute, pred)
	}
	kept, matched := r.partition(elements, pred)
	if matched == 0 {
		return scimerrors.ErrNoTarget(`"path" filter matched no elements`)
	}
	doc[key] = kept
	return nil
}

func (r *engine) removeMemberSub(elements []any, sub string, pred predicate) error {
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

func (r *engine) partition(elements []any, pred predicate) ([]any, int) {
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

func (r *engine) writePath(doc map[string]any, w valueWrite, attr *core.Attribute) error {
	value := r.shape(w.value, isMultiValued(attr))
	if w.path.SubAttribute == "" {
		r.setKey(doc, w.path.Name, value, w.appendMode)
		return nil
	}
	key := object(doc).key(w.path.Name)
	nested, ok := doc[key].(map[string]any)
	if !ok {
		if doc[key] != nil {
			return scimerrors.ErrInvalidPath(`"path" targets a non-complex attribute`)
		}
		nested = map[string]any{}
	}
	r.setKey(nested, w.path.SubAttribute, value, w.appendMode)
	doc[key] = nested
	return nil
}

func (r *engine) guard(path filter.Path, doc map[string]any) (*core.Attribute, error) {
	attr, err := r.attrFor(path)
	if err != nil {
		return nil, err
	}
	return attr, r.checkMutability(attr, r.attrExists(doc, path))
}

func (r *engine) guardParent(path filter.Path, doc map[string]any) (*core.Attribute, error) {
	parent, err := r.parentAttr(path)
	if err != nil {
		return parent, err
	}
	base := path
	base.SubAttribute = ""
	return parent, r.checkMutability(parent, r.attrExists(doc, base))
}

func (r *engine) checkMutability(attr *core.Attribute, present bool) error {
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

func (r *engine) attrFor(path filter.Path) (*core.Attribute, error) {
	if len(r.schemas) == 0 {
		return permissiveAttr, nil
	}
	return r.resolveAttr(path)
}

func (r *engine) parentAttr(path filter.Path) (*core.Attribute, error) {
	base := path
	base.SubAttribute = ""
	return r.attrFor(base)
}

func subAttr(attr *core.Attribute, name string) *core.Attribute {
	if sub := attr.SubAttribute(name); sub != nil {
		return sub
	}
	return permissiveAttr
}

func isMultiValued(attr *core.Attribute) bool {
	return attr.MultiValued
}

func (r *engine) resolveAttr(path filter.Path) (*core.Attribute, error) {
	schema := r.selectSchema(path.URI)
	if schema == nil {
		return nil, scimerrors.ErrInvalidPath(strconv.Quote(path.URI) + " is not a known schema")
	}
	attr, ok := schema.Resolve(path.Name)
	if !ok {
		return nil, scimerrors.ErrInvalidPath(strconv.Quote(path.Name) + " is not a known attribute")
	}
	if path.SubAttribute == "" {
		return attr, nil
	}
	sub := attr.SubAttribute(path.SubAttribute)
	if sub == nil {
		return nil, scimerrors.ErrInvalidPath(strconv.Quote(path.SubAttribute) + " is not a known attribute")
	}
	return sub, nil
}

func (r *engine) selectSchema(uri string) *core.Schema {
	if uri == "" {
		if len(r.schemas) == 0 {
			return nil
		}
		return r.schemas[0]
	}
	for _, schema := range r.schemas {
		if string(schema.ID) == uri {
			return schema
		}
	}
	return nil
}

func (r *engine) setKey(container map[string]any, key string, value any, appendMode bool) {
	existing := object(container).key(key)
	if appendMode {
		if before, ok := container[existing].([]any); ok {
			container[existing] = append(before, r.members(value)...)
			return
		}
	}
	container[existing] = value
}

func (r *engine) members(value any) []any {
	if list, ok := value.([]any); ok {
		return list
	}
	return []any{value}
}

// RFC 7644 3.5.2.1 - a value written to a multi-valued attribute is an array.
func (r *engine) shape(value any, multiValued bool) any {
	if !multiValued {
		return value
	}
	return r.members(value)
}

func (r *engine) attrExists(doc map[string]any, path filter.Path) bool {
	if path.SubAttribute == "" {
		return object(doc).has(path.Name)
	}
	if nested, ok := doc[object(doc).key(path.Name)].(map[string]any); ok {
		return object(nested).has(path.SubAttribute)
	}
	return false
}

func (r *engine) skipOrFail(err error) error {
	if errors.Is(err, errSkip) {
		return nil
	}
	return err
}

func (r *engine) decodeValue(raw json.RawMessage) (any, error) {
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

func (r *engine) decodeMap(raw []byte) (map[string]any, error) {
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

func (r *engine) writeBack(resource any, doc []byte) error {
	value := reflect.ValueOf(resource)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return scimerrors.ErrInternal("resource must be a non-nil pointer")
	}
	elem := value.Elem()
	fresh := reflect.New(elem.Type())
	if err := json.Unmarshal(doc, fresh.Interface()); err != nil {
		return scimerrors.ErrInternal("could not decode the patched resource")
	}
	if elem.CanSet() {
		elem.Set(fresh.Elem())
	}
	return nil
}

func (r *engine) cloneMap(source map[string]any) map[string]any {
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = r.cloneValue(value)
	}
	return cloned
}

func (r *engine) cloneValue(value any) any {
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
