package protocol

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
)

var errSkip = errors.New("scim: skip readOnly attribute")

// PatchRequest is the body of a PATCH request, per RFC 7644, Section 3.5.2.
type PatchRequest struct {
	Schemas    []core.SchemaURI `json:"schemas"`
	Operations []PatchOperation `json:"Operations"`
}

// Apply applies the operations to resource atomically, per RFC 7644, Section 3.5.2.
func (r *PatchRequest) Apply(resource any, schemas []*core.Schema) error {
	if doc, ok := resource.(map[string]any); ok {
		return r.applyToMap(doc, schemas)
	}

	raw, err := json.Marshal(resource)
	if err != nil {
		return ErrInvalidSyntax("resource cannot be encoded")
	}
	doc, err := r.decodeMap(raw)
	if err != nil {
		return ErrInvalidSyntax("resource is not a JSON object")
	}
	if err := r.applyToMap(doc, schemas); err != nil {
		return err
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return ErrInternal("could not encode the patched resource")
	}
	return r.writeBack(resource, out)
}

func (r *PatchRequest) applyToMap(doc map[string]any, schemas []*core.Schema) error {
	working := r.cloneMap(doc)
	for _, op := range r.Operations {
		if err := r.applyOp(working, op, schemas); err != nil {
			return err
		}
	}
	for key := range doc {
		delete(doc, key)
	}
	maps.Copy(doc, working)
	return nil
}

func (r *PatchRequest) applyOp(doc map[string]any, op PatchOperation, schemas []*core.Schema) error {
	switch PatchOp(strings.ToLower(string(op.Op))) {
	case PatchOpAdd:
		return r.applyWrite(doc, op, PatchOpAdd, schemas)
	case PatchOpReplace:
		return r.applyWrite(doc, op, PatchOpReplace, schemas)
	case PatchOpRemove:
		return r.applyRemove(doc, op, schemas)
	default:
		return ErrInvalidSyntax(`"op" must be "add", "remove", or "replace"`)
	}
}

func (r *PatchRequest) applyWrite(doc map[string]any, op PatchOperation, kind PatchOp, schemas []*core.Schema) error {
	if op.Path == "" {
		return r.applyMerge(doc, op.Value, kind, schemas)
	}

	path, err := filter.NewPath(op.Path)
	if err != nil {
		return ErrInvalidPath(err.Error())
	}
	if len(op.Value) == 0 {
		return ErrInvalidValue(`"value" is required for "add" and "replace"`)
	}
	value, err := r.decodeValue(op.Value)
	if err != nil {
		return err
	}

	w := valueWrite{path: path, value: value, appendMode: kind == PatchOpAdd}
	if path.ValueFilter != nil {
		return r.applyValueWrite(doc, w, schemas)
	}
	attr, err := r.guard(schemas, path, doc)
	if err != nil {
		return r.skipOrFail(err)
	}
	return r.writePath(doc, w, attr)
}

func (r *PatchRequest) applyMerge(doc map[string]any, raw json.RawMessage, kind PatchOp, schemas []*core.Schema) error {
	values, err := r.decodeMap(raw)
	if err != nil {
		return ErrInvalidValue(`"value" must be an object when "path" is omitted`)
	}
	appendMode := kind == PatchOpAdd
	for key, value := range values {
		attr, err := r.guard(schemas, r.topLevelPath(key), doc)
		if err != nil {
			if errors.Is(err, errSkip) {
				continue
			}
			return err
		}
		r.setKey(doc, key, r.shape(value, r.isMultiValued(attr)), appendMode)
	}
	return nil
}

func (r *PatchRequest) topLevelPath(name string) filter.Path {
	var path filter.Path
	path.Name = name
	return path
}

func (r *PatchRequest) applyRemove(doc map[string]any, op PatchOperation, schemas []*core.Schema) error {
	if op.Path == "" {
		return ErrNoTarget(`"remove" requires a "path"`)
	}
	path, err := filter.NewPath(op.Path)
	if err != nil {
		return ErrInvalidPath(err.Error())
	}
	if path.ValueFilter != nil {
		return r.applyValueRemove(doc, path, schemas)
	}
	if err := r.enforce(schemas, path, doc); err != nil {
		return r.skipOrFail(err)
	}
	r.removePath(doc, path)
	return nil
}

func (r *PatchRequest) removePath(doc map[string]any, path filter.Path) {
	if path.SubAttribute == "" {
		delete(doc, r.lookupKey(doc, path.Name))
		return
	}
	key := r.lookupKey(doc, path.Name)
	if nested, ok := doc[key].(map[string]any); ok {
		delete(nested, r.lookupKey(nested, path.SubAttribute))
	}
}

func (r *PatchRequest) applyValueWrite(doc map[string]any, w valueWrite, schemas []*core.Schema) error {
	parent, err := r.parentAttr(schemas, w.path)
	if err != nil {
		return err
	}
	pred, err := r.compile(w.path.ValueFilter, parent)
	if err != nil {
		return err
	}
	attr, err := r.attrFor(schemas, w.path)
	if err != nil {
		return err
	}

	key := r.lookupKey(doc, w.path.Name)
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
		return ErrNoTarget(`"path" filter matched no elements`)
	}
	doc[key] = elements
	return nil
}

func (r *PatchRequest) writeMember(member map[string]any, w valueWrite, attr *core.Attribute) error {
	if w.path.SubAttribute != "" {
		if attr != nil {
			if err := r.checkMutability(attr, r.hasKey(member, w.path.SubAttribute)); err != nil {
				return err
			}
		}
		r.setKey(member, w.path.SubAttribute, r.shape(w.value, r.isMultiValued(attr)), w.appendMode)
		return nil
	}
	object, ok := w.value.(map[string]any)
	if !ok {
		return ErrInvalidValue(`"value" must be an object when "path" has no sub-attribute`)
	}
	return r.mergeMember(member, object, w.appendMode, attr)
}

func (r *PatchRequest) mergeMember(member, object map[string]any, appendMode bool, attr *core.Attribute) error {
	for key, value := range object {
		sub := r.subAttr(attr, key)
		if sub != nil {
			if err := r.checkMutability(sub, r.hasKey(member, key)); err != nil {
				if errors.Is(err, errSkip) {
					continue
				}
				return err
			}
		}
		r.setKey(member, key, r.shape(value, r.isMultiValued(sub)), appendMode)
	}
	return nil
}

func (r *PatchRequest) applyValueRemove(doc map[string]any, path filter.Path, schemas []*core.Schema) error {
	parent, err := r.parentAttr(schemas, path)
	if err != nil {
		return err
	}
	pred, err := r.compile(path.ValueFilter, parent)
	if err != nil {
		return err
	}
	if err := r.enforce(schemas, path, doc); err != nil {
		return r.skipOrFail(err)
	}

	key := r.lookupKey(doc, path.Name)
	elements, _ := doc[key].([]any)
	if path.SubAttribute != "" {
		return r.removeMemberSub(elements, path.SubAttribute, pred)
	}
	kept, matched := r.partition(elements, pred)
	if matched == 0 {
		return ErrNoTarget(`"path" filter matched no elements`)
	}
	doc[key] = kept
	return nil
}

func (r *PatchRequest) removeMemberSub(elements []any, sub string, pred predicate) error {
	matched := 0
	for _, element := range elements {
		if member, ok := element.(map[string]any); ok && pred(member) {
			delete(member, r.lookupKey(member, sub))
			matched++
		}
	}
	if matched == 0 {
		return ErrNoTarget(`"path" filter matched no elements`)
	}
	return nil
}

func (r *PatchRequest) partition(elements []any, pred predicate) ([]any, int) {
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

func (r *PatchRequest) writePath(doc map[string]any, w valueWrite, attr *core.Attribute) error {
	value := r.shape(w.value, r.isMultiValued(attr))
	if w.path.SubAttribute == "" {
		r.setKey(doc, w.path.Name, value, w.appendMode)
		return nil
	}
	key := r.lookupKey(doc, w.path.Name)
	nested, ok := doc[key].(map[string]any)
	if !ok {
		if doc[key] != nil {
			return ErrInvalidPath(`"path" targets a non-complex attribute`)
		}
		nested = map[string]any{}
	}
	r.setKey(nested, w.path.SubAttribute, value, w.appendMode)
	doc[key] = nested
	return nil
}

func (r *PatchRequest) enforce(schemas []*core.Schema, path filter.Path, doc map[string]any) error {
	_, err := r.guard(schemas, path, doc)
	return err
}

func (r *PatchRequest) guard(schemas []*core.Schema, path filter.Path, doc map[string]any) (*core.Attribute, error) {
	attr, err := r.attrFor(schemas, path)
	if err != nil || attr == nil {
		return nil, err
	}
	return attr, r.checkMutability(attr, r.attrExists(doc, path))
}

func (r *PatchRequest) checkMutability(attr *core.Attribute, present bool) error {
	switch attr.Mutability {
	case core.MutabilityReadOnly:
		return errSkip
	case core.MutabilityImmutable:
		if present {
			return ErrMutability(strconv.Quote(attr.Name) + " is immutable")
		}
	}
	return nil
}

func (r *PatchRequest) attrFor(schemas []*core.Schema, path filter.Path) (*core.Attribute, error) {
	if len(schemas) == 0 {
		return nil, nil
	}
	return r.resolveAttr(schemas, path)
}

func (r *PatchRequest) parentAttr(schemas []*core.Schema, path filter.Path) (*core.Attribute, error) {
	base := path
	base.SubAttribute = ""
	return r.attrFor(schemas, base)
}

func (r *PatchRequest) subAttr(attr *core.Attribute, name string) *core.Attribute {
	if attr == nil {
		return nil
	}
	return attr.SubAttribute(name)
}

func (r *PatchRequest) isMultiValued(attr *core.Attribute) bool {
	return attr != nil && attr.MultiValued
}

func (r *PatchRequest) isCaseExact(attr *core.Attribute, name string) bool {
	sub := r.subAttr(attr, name)
	return sub != nil && sub.CaseExact
}

func (r *PatchRequest) resolveAttr(schemas []*core.Schema, path filter.Path) (*core.Attribute, error) {
	schema := r.selectSchema(schemas, path.URI)
	if schema == nil {
		return nil, ErrInvalidPath(strconv.Quote(path.URI) + " is not a known schema")
	}
	attr, ok := schema.Resolve(path.Name)
	if !ok {
		return nil, ErrInvalidPath(strconv.Quote(path.Name) + " is not a known attribute")
	}
	if path.SubAttribute == "" {
		return attr, nil
	}
	sub := attr.SubAttribute(path.SubAttribute)
	if sub == nil {
		return nil, ErrInvalidPath(strconv.Quote(path.SubAttribute) + " is not a known attribute")
	}
	return sub, nil
}

func (r *PatchRequest) selectSchema(schemas []*core.Schema, uri string) *core.Schema {
	if uri == "" {
		if len(schemas) == 0 {
			return nil
		}
		return schemas[0]
	}
	for _, schema := range schemas {
		if string(schema.ID) == uri {
			return schema
		}
	}
	return nil
}

func (r *PatchRequest) compile(node *filter.Node, attr *core.Attribute) (predicate, error) {
	switch {
	case node == nil:
		return nil, ErrInvalidPath("empty value filter")
	case node.Not():
		return r.compileNot(node, attr)
	case node.Left() != nil:
		return r.compileBinary(node, attr)
	default:
		return r.compileLeaf(node, attr), nil
	}
}

func (r *PatchRequest) compileNot(node *filter.Node, attr *core.Attribute) (predicate, error) {
	inner, err := r.compile(node.Operand(), attr)
	if err != nil {
		return nil, err
	}
	return func(m map[string]any) bool { return !inner(m) }, nil
}

func (r *PatchRequest) compileBinary(node *filter.Node, attr *core.Attribute) (predicate, error) {
	left, err := r.compile(node.Left(), attr)
	if err != nil {
		return nil, err
	}
	right, err := r.compile(node.Right(), attr)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(node.Operator(), "or") {
		return func(m map[string]any) bool { return left(m) || right(m) }, nil
	}
	return func(m map[string]any) bool { return left(m) && right(m) }, nil
}

func (r *PatchRequest) compileLeaf(node *filter.Node, attr *core.Attribute) predicate {
	l := leaf{
		key:       node.AttrPath().Name,
		op:        strings.ToLower(node.Operator()),
		want:      node.Value(),
		caseExact: r.isCaseExact(attr, node.AttrPath().Name),
	}
	return func(m map[string]any) bool { return r.matchOne(m, l) }
}

func (r *PatchRequest) matchOne(member map[string]any, l leaf) bool {
	got, ok := r.lookupCI(member, l.key)
	if l.op == "pr" {
		return ok && got != nil
	}
	if !ok || got == nil {
		return false
	}
	return r.compareValues(filter.Operator(l.op), got, l.want, l.caseExact)
}

func (r *PatchRequest) compareValues(op filter.Operator, got, want any, caseExact bool) bool {
	if gs, ok := got.(string); ok {
		ws, ok := want.(string)
		return ok && r.compareStrings(op, gs, ws, caseExact)
	}
	if gb, ok := got.(bool); ok {
		wb, ok := want.(bool)
		return ok && r.compareBools(op, gb, wb)
	}
	gf, gok := r.toFloat(got)
	wf, wok := r.toFloat(want)
	return gok && wok && r.compareNumbers(op, gf, wf)
}

func (r *PatchRequest) compareStrings(op filter.Operator, got, want string, caseExact bool) bool {
	if !caseExact {
		got, want = strings.ToLower(got), strings.ToLower(want)
	}
	switch op {
	case filter.OpEquals:
		return got == want
	case filter.OpNotEquals:
		return got != want
	case filter.OpContains:
		return strings.Contains(got, want)
	case filter.OpStartsWith:
		return strings.HasPrefix(got, want)
	case filter.OpEndsWith:
		return strings.HasSuffix(got, want)
	case filter.OpGreaterThan:
		return got > want
	case filter.OpLessThan:
		return got < want
	case filter.OpGreaterThanEquals:
		return got >= want
	case filter.OpLessThanEquals:
		return got <= want
	}
	return false
}

func (r *PatchRequest) compareBools(op filter.Operator, got, want bool) bool {
	switch op {
	case filter.OpEquals:
		return got == want
	case filter.OpNotEquals:
		return got != want
	}
	return false
}

func (r *PatchRequest) compareNumbers(op filter.Operator, got, want float64) bool {
	switch op {
	case filter.OpEquals:
		return got == want
	case filter.OpNotEquals:
		return got != want
	case filter.OpGreaterThan:
		return got > want
	case filter.OpLessThan:
		return got < want
	case filter.OpGreaterThanEquals:
		return got >= want
	case filter.OpLessThanEquals:
		return got <= want
	}
	return false
}

func (r *PatchRequest) toFloat(value any) (float64, bool) {
	switch n := value.(type) {
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case float64:
		return n, true
	case int64:
		return float64(n), true
	}
	return 0, false
}

func (r *PatchRequest) setKey(container map[string]any, key string, value any, appendMode bool) {
	existing := r.lookupKey(container, key)
	if appendMode {
		if before, ok := container[existing].([]any); ok {
			container[existing] = append(before, r.members(value)...)
			return
		}
	}
	container[existing] = value
}

func (r *PatchRequest) members(value any) []any {
	if list, ok := value.([]any); ok {
		return list
	}
	return []any{value}
}

// RFC 7644 3.5.2.1 - a value written to a multi-valued attribute is an array.
func (r *PatchRequest) shape(value any, multiValued bool) any {
	if _, ok := value.([]any); multiValued && !ok {
		return []any{value}
	}
	return value
}

func (r *PatchRequest) lookupKey(container map[string]any, key string) string {
	if _, ok := container[key]; ok {
		return key
	}
	for candidate := range container {
		if strings.EqualFold(candidate, key) {
			return candidate
		}
	}
	return key
}

func (r *PatchRequest) lookupCI(container map[string]any, key string) (any, bool) {
	value, ok := container[r.lookupKey(container, key)]
	return value, ok
}

func (r *PatchRequest) hasKey(container map[string]any, key string) bool {
	_, ok := container[r.lookupKey(container, key)]
	return ok
}

func (r *PatchRequest) attrExists(doc map[string]any, path filter.Path) bool {
	if path.SubAttribute == "" {
		return r.hasKey(doc, path.Name)
	}
	if nested, ok := doc[r.lookupKey(doc, path.Name)].(map[string]any); ok {
		return r.hasKey(nested, path.SubAttribute)
	}
	return false
}

func (r *PatchRequest) skipOrFail(err error) error {
	if errors.Is(err, errSkip) {
		return nil
	}
	return err
}

func (r *PatchRequest) decodeValue(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, ErrInvalidValue(`"value" is not valid JSON`)
	}
	return value, nil
}

func (r *PatchRequest) decodeMap(raw []byte) (map[string]any, error) {
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

func (r *PatchRequest) writeBack(resource any, doc []byte) error {
	value := reflect.ValueOf(resource)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return ErrInternal("resource must be a non-nil pointer")
	}
	elem := value.Elem()
	fresh := reflect.New(elem.Type())
	if err := json.Unmarshal(doc, fresh.Interface()); err != nil {
		return ErrInternal("could not decode the patched resource")
	}
	if elem.CanSet() {
		elem.Set(fresh.Elem())
	}
	return nil
}

func (r *PatchRequest) cloneMap(source map[string]any) map[string]any {
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = r.cloneValue(value)
	}
	return cloned
}

func (r *PatchRequest) cloneValue(value any) any {
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
