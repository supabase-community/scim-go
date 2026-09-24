package patch

import (
	"bytes"
	"strconv"
	"strings"

	"github.com/supabase-community/scim-go/internal/decode"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

var permissiveAttr = &core.Attribute{}

type patcher struct {
	schemas core.Schemas
}

func (p *patcher) run(root core.Object, ops []Operation) error {
	for _, op := range ops {
		before := primariesOf(root)
		if err := p.apply(root, op); err != nil {
			return err
		}
		before.demote(root)
	}
	return nil
}

func (p *patcher) apply(root core.Object, op Operation) error {
	switch kind := Op(strings.ToLower(string(op.Op))); kind {
	case OpAdd, OpReplace:
		return p.write(root, kind, op)
	case OpRemove:
		return p.remove(root, op)
	default:
		return scimerrors.ErrInvalidSyntax(`"op" must be "add", "remove", or "replace"`)
	}
}

func (p *patcher) write(root core.Object, kind Op, op Operation) error {
	if op.Path == "" {
		values, err := decode.JSON[map[string]any](bytes.NewReader(op.Value))
		if err != nil || values == nil {
			return scimerrors.ErrInvalidValue(`"value" must be an object when "path" is omitted`)
		}
		return p.mergeRoot(root, values, kind)
	}

	path, err := filter.NewPath(op.Path)
	if err != nil {
		return scimerrors.ErrInvalidPath(err.Error())
	}
	if len(op.Value) == 0 {
		return scimerrors.ErrInvalidValue(`"value" is required for "add" and "replace"`)
	}
	value, err := decode.JSON[any](bytes.NewReader(op.Value))
	if err != nil {
		return scimerrors.ErrInvalidValue(`"value" is not valid JSON`)
	}
	if path.ValueFilter != nil {
		return p.valueWrite(root, path, kind, value)
	}
	return p.writeAt(root, path, kind, value)
}

func (p *patcher) writeAt(root core.Object, path filter.Path, kind Op, value any) error {
	attr, err := resolve(p.schemas, path)
	if err != nil {
		return err
	}
	if err := gate(attr); err != nil {
		return err
	}
	container, err := p.container(root, path)
	if err != nil {
		return err
	}
	target, key, err := locate(container, path)
	if err != nil {
		return err
	}
	if values, ok := value.(map[string]any); ok && attr.Type == core.TypeComplex && !attr.MultiValued {
		nested, err := child(target, key)
		if err != nil {
			return err
		}
		return p.mergeMember(nested, values, attr, kind)
	}
	set(target, key, shaped(value, attr.MultiValued), kind)
	return nil
}

func (p *patcher) remove(root core.Object, op Operation) error {
	if op.Path == "" {
		return scimerrors.ErrNoTarget(`"remove" requires a "path"`)
	}
	path, err := filter.NewPath(op.Path)
	if err != nil {
		return scimerrors.ErrInvalidPath(err.Error())
	}
	if path.ValueFilter != nil {
		return p.valueRemove(root, path)
	}
	attr, err := resolve(p.schemas, path)
	if err != nil {
		return err
	}
	container := p.within(root, path)
	if err := gate(attr); err != nil {
		return err
	}
	if path.SubAttribute == "" {
		container.Remove(path.Name)
		return nil
	}
	return removeSub(container, path)
}

func (p *patcher) valueWrite(root core.Object, path filter.Path, kind Op, value any) error {
	parent, err := resolve(p.schemas, base(path))
	if err != nil {
		return err
	}
	if err := gate(parent); err != nil {
		return err
	}
	pred, err := compile(parent, path.ValueFilter)
	if err != nil {
		return err
	}
	writeOne, err := p.memberWriter(path, parent, kind, value)
	if err != nil {
		return err
	}
	container := p.within(root, path)
	elements, _ := container.Get(path.Name).([]any)
	matched, err := eachMatch(elements, pred, writeOne)
	if err != nil {
		return err
	}
	if matched == 0 {
		return scimerrors.ErrNoTarget(`"path" filter matched no elements`)
	}
	container.Set(path.Name, elements)
	return nil
}

func (p *patcher) memberWriter(path filter.Path, parent *core.Attribute, kind Op, value any) (func(core.Object) error, error) {
	if path.SubAttribute == "" {
		values, ok := value.(map[string]any)
		if !ok {
			return nil, scimerrors.ErrInvalidValue(`"value" must be an object when "path" has no sub-attribute`)
		}
		return func(member core.Object) error {
			return p.mergeMember(member, values, parent, kind)
		}, nil
	}
	attr, err := resolve(p.schemas, path)
	if err != nil {
		return nil, err
	}
	return func(member core.Object) error {
		if err := gate(attr); err != nil {
			return err
		}
		set(member, path.SubAttribute, shaped(value, attr.MultiValued), kind)
		return nil
	}, nil
}

func (p *patcher) valueRemove(root core.Object, path filter.Path) error {
	parent, err := resolve(p.schemas, base(path))
	if err != nil {
		return err
	}
	container := p.within(root, path)
	if err := gate(parent); err != nil {
		return err
	}
	pred, err := compile(parent, path.ValueFilter)
	if err != nil {
		return err
	}
	elements, _ := container.Get(path.Name).([]any)
	if path.SubAttribute != "" {
		return p.clearSub(container, path, elements, pred)
	}
	return dropMembers(container, path.Name, elements, pred)
}

func (p *patcher) clearSub(container core.Object, path filter.Path, elements []any, pred predicate) error {
	attr, err := resolve(p.schemas, path)
	if err != nil {
		return err
	}
	if err := gate(attr); err != nil {
		return err
	}
	matched, _ := eachMatch(elements, pred, func(member core.Object) error {
		member.Remove(path.SubAttribute)
		return nil
	})
	if matched == 0 {
		return scimerrors.ErrNoTarget(`"path" filter matched no elements`)
	}
	return nil
}

// RFC 7644 Section 3.5.2: without a "path" the value names attributes, possibly URN-qualified or grouped under an extension URN.
func (p *patcher) mergeRoot(root core.Object, values map[string]any, kind Op) error {
	for key, value := range values {
		if schema := p.schemas.Lookup(core.SchemaURI(key)); schema != nil {
			if err := p.mergeSchema(root, schema, value, kind); err != nil {
				return err
			}
			continue
		}
		if err := p.writeAt(root, p.keyPath(key), kind, value); err != nil {
			return err
		}
	}
	return nil
}

func (p *patcher) mergeSchema(root core.Object, schema *core.Schema, value any, kind Op) error {
	values, ok := value.(map[string]any)
	if !ok {
		return scimerrors.ErrInvalidValue(strconv.Quote(string(schema.ID)) + " must be an object")
	}
	for name, item := range values {
		path := filter.Path{URI: string(schema.ID), Name: name}
		if err := p.writeAt(root, path, kind, item); err != nil {
			return err
		}
	}
	return nil
}

func (p *patcher) keyPath(key string) filter.Path {
	if len(p.schemas) > 0 {
		if path, err := filter.NewAttrPath(key); err == nil {
			return filter.Path{AttrPath: path}
		}
	}
	return filter.Path{Name: key}
}

// RFC 7644 Section 3.5.2.3: sub-attributes that are not specified in the "value" parameter are left unchanged.
func (p *patcher) mergeMember(target core.Object, values map[string]any, parent *core.Attribute, kind Op) error {
	for key, value := range values {
		attr := subAttr(parent, key)
		if err := gate(attr); err != nil {
			return err
		}
		set(target, key, shaped(value, attr.MultiValued), kind)
	}
	return nil
}

// RFC 7643 Section 3.3: extension attributes live in an object keyed by the extension URN.
func (p *patcher) extension(path filter.Path) *core.Schema {
	if path.URI == "" {
		return nil
	}
	if schema := p.schemas.Lookup(core.SchemaURI(path.URI)); p.schemas.IsExtension(schema) {
		return schema
	}
	return nil
}

func (p *patcher) within(root core.Object, path filter.Path) core.Object {
	if schema := p.extension(path); schema != nil {
		nested, _ := root.Get(string(schema.ID)).(map[string]any)
		return nested
	}
	return root
}

func (p *patcher) container(root core.Object, path filter.Path) (core.Object, error) {
	if schema := p.extension(path); schema != nil {
		return child(root, string(schema.ID))
	}
	return root, nil
}

func removeSub(root core.Object, path filter.Path) error {
	switch nested := root.Get(path.Name).(type) {
	case map[string]any:
		core.Object(nested).Remove(path.SubAttribute)
		return nil
	case []any:
		matched, _ := eachMatch(nested, func(map[string]any) bool { return true }, func(member core.Object) error {
			member.Remove(path.SubAttribute)
			return nil
		})
		if matched == 0 {
			return scimerrors.ErrNoTarget(`"path" matched no elements`)
		}
		return nil
	}
	return nil
}

func dropMembers(container core.Object, name string, elements []any, pred predicate) error {
	kept := make([]any, 0, len(elements))
	matched := 0
	for _, element := range elements {
		if member, ok := element.(map[string]any); ok && pred(member) {
			matched++
			continue
		}
		kept = append(kept, element)
	}
	if matched == 0 {
		return scimerrors.ErrNoTarget(`"path" filter matched no elements`)
	}
	container.Set(name, kept)
	return nil
}

func locate(root core.Object, path filter.Path) (core.Object, string, error) {
	if path.SubAttribute == "" {
		return root, path.Name, nil
	}
	nested, err := child(root, path.Name)
	if err != nil {
		return nil, "", err
	}
	return nested, path.SubAttribute, nil
}

func resolve(schemas core.Schemas, path filter.Path) (*core.Attribute, error) {
	if len(schemas) == 0 {
		return permissiveAttr, nil
	}
	uri := core.SchemaURI(path.URI)
	if schemas.Lookup(uri) == nil {
		return nil, scimerrors.ErrInvalidPath(strconv.Quote(path.URI) + " is not a known schema")
	}
	attr, ok := schemas.Resolve(uri, path.Name, path.SubAttribute)
	if !ok {
		return nil, scimerrors.ErrInvalidPath(strconv.Quote(path.String()) + " is not a known attribute")
	}
	return attr, nil
}

func subAttr(parent *core.Attribute, name string) *core.Attribute {
	if sub := parent.SubAttribute(name); sub != nil {
		return sub
	}
	return permissiveAttr
}

func base(path filter.Path) filter.Path {
	path.SubAttribute = ""
	return path
}

func gate(attr *core.Attribute) error {
	if attr.Mutability == core.MutabilityReadOnly {
		return scimerrors.ErrMutability(strconv.Quote(attr.Name) + " is readOnly")
	}
	return nil
}

func eachMatch(elements []any, pred predicate, fn func(core.Object) error) (int, error) {
	matched := 0
	for _, element := range elements {
		member, ok := element.(map[string]any)
		if !ok || !pred(member) {
			continue
		}
		matched++
		if err := fn(core.Object(member)); err != nil {
			return matched, err
		}
	}
	return matched, nil
}

// RFC 7644 3.5.2.1 - a value written to a multi-valued attribute is an array.
func shaped(value any, multiValued bool) any {
	if !multiValued {
		return value
	}
	if list, ok := value.([]any); ok {
		return list
	}
	return []any{value}
}
