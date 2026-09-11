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

// errSkip marks a readOnly attribute that is silently ignored, per RFC 7643, Section 7.
var errSkip = errors.New("scim: skip readOnly attribute")

// Apply applies the operations to resource atomically, per RFC 7644, Section 3.5.2.
func (r *PatchRequest) Apply(resource any, schemas []*core.Schema) error {
	if doc, ok := resource.(map[string]any); ok {
		return r.applyToMap(doc, schemas)
	}

	raw, err := json.Marshal(resource)
	if err != nil {
		return ErrInvalidSyntax("resource cannot be encoded")
	}
	doc, err := decodeMap(raw)
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
	return writeBack(resource, out)
}

func (r *PatchRequest) applyToMap(doc map[string]any, schemas []*core.Schema) error {
	working := cloneMap(doc)
	for _, op := range r.Operations {
		if err := applyOp(working, op, schemas); err != nil {
			return err
		}
	}
	for key := range doc {
		delete(doc, key)
	}
	maps.Copy(doc, working)
	return nil
}

func applyOp(doc map[string]any, op PatchOperation, schemas []*core.Schema) error {
	switch PatchOp(strings.ToLower(string(op.Op))) {
	case PatchOpAdd:
		return applyWrite(doc, op, PatchOpAdd, schemas)
	case PatchOpReplace:
		return applyWrite(doc, op, PatchOpReplace, schemas)
	case PatchOpRemove:
		return applyRemove(doc, op, schemas)
	default:
		return ErrInvalidSyntax(`"op" must be "add", "remove", or "replace"`)
	}
}

func applyWrite(doc map[string]any, op PatchOperation, kind PatchOp, schemas []*core.Schema) error {
	if op.Path == "" {
		return applyMerge(doc, op.Value, kind, schemas)
	}

	path, err := filter.ParsePath(op.Path)
	if err != nil {
		return ErrInvalidPath(err.Error())
	}
	if len(op.Value) == 0 {
		return ErrInvalidValue(`"value" is required for "add" and "replace"`)
	}
	value, err := decodeValue(op.Value)
	if err != nil {
		return err
	}

	appendMode := kind == PatchOpAdd
	if path.ValueFilter != nil {
		return applyValueWrite(doc, valueWrite{path: path, value: value, appendMode: appendMode}, schemas)
	}
	if err := enforce(schemas, path, doc); err != nil {
		return skipOrFail(err)
	}
	return writePath(doc, path, value, appendMode)
}

func applyMerge(doc map[string]any, raw json.RawMessage, kind PatchOp, schemas []*core.Schema) error {
	values, err := decodeMap(raw)
	if err != nil {
		return ErrInvalidValue(`"value" must be an object when "path" is omitted`)
	}
	appendMode := kind == PatchOpAdd
	for key, value := range values {
		if err := enforce(schemas, topLevelPath(key), doc); err != nil {
			if errors.Is(err, errSkip) {
				continue
			}
			return err
		}
		setKey(doc, key, value, appendMode)
	}
	return nil
}

func topLevelPath(name string) filter.Path {
	var path filter.Path
	path.Name = name
	return path
}

func applyRemove(doc map[string]any, op PatchOperation, schemas []*core.Schema) error {
	if op.Path == "" {
		return ErrNoTarget(`"remove" requires a "path"`)
	}
	path, err := filter.ParsePath(op.Path)
	if err != nil {
		return ErrInvalidPath(err.Error())
	}
	if path.ValueFilter != nil {
		return applyValueRemove(doc, path, schemas)
	}
	if err := enforce(schemas, path, doc); err != nil {
		return skipOrFail(err)
	}
	removePath(doc, path)
	return nil
}

func removePath(doc map[string]any, path filter.Path) {
	if path.SubAttribute == "" {
		delete(doc, lookupKey(doc, path.Name))
		return
	}
	key := lookupKey(doc, path.Name)
	if nested, ok := doc[key].(map[string]any); ok {
		delete(nested, lookupKey(nested, path.SubAttribute))
	}
}

func applyValueWrite(doc map[string]any, w valueWrite, schemas []*core.Schema) error {
	pred, err := compile(w.path.ValueFilter)
	if err != nil {
		return err
	}
	attr, err := valuePathAttr(schemas, w.path)
	if err != nil {
		return err
	}

	key := lookupKey(doc, w.path.Name)
	elements, _ := doc[key].([]any)
	matched := 0
	for _, element := range elements {
		member, ok := element.(map[string]any)
		if !ok || !pred(member) {
			continue
		}
		matched++
		if err := writeMember(member, w, attr); err != nil {
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

func writeMember(member map[string]any, w valueWrite, attr *core.Attribute) error {
	if attr != nil {
		present := w.path.SubAttribute == "" || hasKey(member, w.path.SubAttribute)
		if err := checkMutability(attr, present); err != nil {
			return err
		}
	}
	if w.path.SubAttribute != "" {
		setKey(member, w.path.SubAttribute, w.value, w.appendMode)
		return nil
	}
	object, ok := w.value.(map[string]any)
	if !ok {
		return ErrInvalidValue(`"value" must be an object when "path" has no sub-attribute`)
	}
	for k, v := range object {
		setKey(member, k, v, w.appendMode)
	}
	return nil
}

func applyValueRemove(doc map[string]any, path filter.Path, schemas []*core.Schema) error {
	pred, err := compile(path.ValueFilter)
	if err != nil {
		return err
	}
	if err := enforce(schemas, path, doc); err != nil {
		return skipOrFail(err)
	}

	key := lookupKey(doc, path.Name)
	elements, _ := doc[key].([]any)
	if path.SubAttribute != "" {
		return removeMemberSub(elements, path.SubAttribute, pred)
	}
	kept, matched := partition(elements, pred)
	if matched == 0 {
		return ErrNoTarget(`"path" filter matched no elements`)
	}
	doc[key] = kept
	return nil
}

func removeMemberSub(elements []any, sub string, pred predicate) error {
	matched := 0
	for _, element := range elements {
		if member, ok := element.(map[string]any); ok && pred(member) {
			delete(member, lookupKey(member, sub))
			matched++
		}
	}
	if matched == 0 {
		return ErrNoTarget(`"path" filter matched no elements`)
	}
	return nil
}

func partition(elements []any, pred predicate) ([]any, int) {
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

func writePath(doc map[string]any, path filter.Path, value any, appendMode bool) error {
	if path.SubAttribute == "" {
		setKey(doc, path.Name, value, appendMode)
		return nil
	}
	key := lookupKey(doc, path.Name)
	nested, ok := doc[key].(map[string]any)
	if !ok {
		if doc[key] != nil {
			return ErrInvalidPath(`"path" targets a non-complex attribute`)
		}
		nested = map[string]any{}
	}
	setKey(nested, path.SubAttribute, value, appendMode)
	doc[key] = nested
	return nil
}

// enforce resolves the target attribute and applies mutability, per RFC 7643, Section 7.
func enforce(schemas []*core.Schema, path filter.Path, doc map[string]any) error {
	if len(schemas) == 0 {
		return nil
	}
	attr, err := resolveAttr(schemas, path)
	if err != nil {
		return err
	}
	return checkMutability(attr, attrExists(doc, path))
}

// checkMutability guards readOnly and immutable attributes, per RFC 7643, Section 7.
func checkMutability(attr *core.Attribute, present bool) error {
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

func valuePathAttr(schemas []*core.Schema, path filter.Path) (*core.Attribute, error) {
	if len(schemas) == 0 {
		return nil, nil
	}
	return resolveAttr(schemas, path)
}

func resolveAttr(schemas []*core.Schema, path filter.Path) (*core.Attribute, error) {
	schema := selectSchema(schemas, path.URI)
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

func selectSchema(schemas []*core.Schema, uri string) *core.Schema {
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

// compile builds an in-memory predicate from a valuePath filter, per RFC 7644, Section 3.4.2.2.
func compile(node *filter.Node) (predicate, error) {
	switch {
	case node == nil:
		return nil, ErrInvalidPath("empty value filter")
	case node.Not():
		return compileNot(node)
	case node.Left() != nil:
		return compileBinary(node)
	default:
		return compileLeaf(node), nil
	}
}

func compileNot(node *filter.Node) (predicate, error) {
	inner, err := compile(node.Operand())
	if err != nil {
		return nil, err
	}
	return func(m map[string]any) bool { return !inner(m) }, nil
}

func compileBinary(node *filter.Node) (predicate, error) {
	left, err := compile(node.Left())
	if err != nil {
		return nil, err
	}
	right, err := compile(node.Right())
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(node.Operator(), "or") {
		return func(m map[string]any) bool { return left(m) || right(m) }, nil
	}
	return func(m map[string]any) bool { return left(m) && right(m) }, nil
}

func compileLeaf(node *filter.Node) predicate {
	key := node.AttrPath().Name
	op := strings.ToLower(node.Operator())
	want := node.Value()
	return func(m map[string]any) bool { return matchOne(m, key, op, want) }
}

func matchOne(member map[string]any, key, op string, want any) bool {
	got, ok := lookupCI(member, key)
	if op == "pr" {
		return ok && got != nil
	}
	if !ok || got == nil {
		return false
	}
	return compareValues(op, got, want)
}

func compareValues(op string, got, want any) bool {
	if gs, ok := got.(string); ok {
		ws, ok := want.(string)
		return ok && compareStrings(op, gs, ws)
	}
	if gb, ok := got.(bool); ok {
		wb, ok := want.(bool)
		return ok && compareBools(op, gb, wb)
	}
	gf, gok := toFloat(got)
	wf, wok := toFloat(want)
	return gok && wok && compareNumbers(op, gf, wf)
}

func compareStrings(op, got, want string) bool {
	lowerGot, lowerWant := strings.ToLower(got), strings.ToLower(want)
	switch op {
	case "eq":
		return lowerGot == lowerWant
	case "ne":
		return lowerGot != lowerWant
	case "co":
		return strings.Contains(lowerGot, lowerWant)
	case "sw":
		return strings.HasPrefix(lowerGot, lowerWant)
	case "ew":
		return strings.HasSuffix(lowerGot, lowerWant)
	case "gt":
		return lowerGot > lowerWant
	case "lt":
		return lowerGot < lowerWant
	case "ge":
		return lowerGot >= lowerWant
	case "le":
		return lowerGot <= lowerWant
	}
	return false
}

func compareBools(op string, got, want bool) bool {
	switch op {
	case "eq":
		return got == want
	case "ne":
		return got != want
	}
	return false
}

func compareNumbers(op string, got, want float64) bool {
	switch op {
	case "eq":
		return got == want
	case "ne":
		return got != want
	case "gt":
		return got > want
	case "lt":
		return got < want
	case "ge":
		return got >= want
	case "le":
		return got <= want
	}
	return false
}

func toFloat(value any) (float64, bool) {
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

func setKey(container map[string]any, key string, value any, appendMode bool) {
	existing := lookupKey(container, key)
	if appendMode {
		if before, ok := container[existing].([]any); ok {
			if after, ok := value.([]any); ok {
				container[existing] = append(before, after...)
				return
			}
			container[existing] = append(before, value)
			return
		}
	}
	container[existing] = value
}

func lookupKey(container map[string]any, key string) string {
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

func lookupCI(container map[string]any, key string) (any, bool) {
	value, ok := container[lookupKey(container, key)]
	return value, ok
}

func hasKey(container map[string]any, key string) bool {
	_, ok := container[lookupKey(container, key)]
	return ok
}

func attrExists(doc map[string]any, path filter.Path) bool {
	if path.SubAttribute == "" {
		return hasKey(doc, path.Name)
	}
	if nested, ok := doc[lookupKey(doc, path.Name)].(map[string]any); ok {
		return hasKey(nested, path.SubAttribute)
	}
	return false
}

func skipOrFail(err error) error {
	if errors.Is(err, errSkip) {
		return nil
	}
	return err
}

func decodeValue(raw json.RawMessage) (any, error) {
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

func decodeMap(raw []byte) (map[string]any, error) {
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

func writeBack(resource any, doc []byte) error {
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

func cloneMap(source map[string]any) map[string]any {
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = cloneValue(value)
	}
	return cloned
}

func cloneValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneMap(typed)
	case []any:
		cloned := make([]any, len(typed))
		for i, element := range typed {
			cloned[i] = cloneValue(element)
		}
		return cloned
	default:
		return typed
	}
}
