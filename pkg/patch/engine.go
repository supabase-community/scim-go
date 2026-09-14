package patch

import (
	"encoding/json"
	"reflect"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

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
		return newDocument(r.schemas, doc).applyAll(ops)
	}

	raw, err := json.Marshal(resource)
	if err != nil {
		return scimerrors.ErrInvalidSyntax("resource cannot be encoded")
	}
	d := newDocument(r.schemas, nil)
	doc, err := d.decodeMap(raw)
	if err != nil {
		return scimerrors.ErrInvalidSyntax("resource is not a JSON object")
	}
	d.resource = doc
	if err := d.applyAll(ops); err != nil {
		return err
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return scimerrors.ErrInternal("could not encode the patched resource")
	}
	return r.writeBack(resource, out)
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
