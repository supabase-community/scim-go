package patch

import (
	"errors"
	"strconv"
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

var (
	permissiveAttr = &core.Attribute{}
	errSkip        = errors.New("scim: skip readOnly attribute")
)

type patcher struct {
	schemas []*core.Schema
}

func (p *patcher) run(root object, ops []Operation) error {
	for _, op := range ops {
		if err := p.apply(root, op); err != nil {
			return err
		}
	}
	return nil
}

func (p *patcher) apply(root object, op Operation) error {
	switch Op(strings.ToLower(string(op.Op))) {
	case OpAdd:
		return p.write(root, op, true)
	case OpReplace:
		return p.write(root, op, false)
	case OpRemove:
		return p.remove(root, op)
	default:
		return scimerrors.ErrInvalidSyntax(`"op" must be "add", "remove", or "replace"`)
	}
}

func (p *patcher) write(root object, op Operation, appendMode bool) error {
	if op.Path == "" {
		values, err := decode[map[string]any](op.Value)
		if err != nil || values == nil {
			return scimerrors.ErrInvalidValue(`"value" must be an object when "path" is omitted`)
		}
		return p.mergeAll(root, values, nil, appendMode)
	}

	path, err := filter.NewPath(op.Path)
	if err != nil {
		return scimerrors.ErrInvalidPath(err.Error())
	}
	if len(op.Value) == 0 {
		return scimerrors.ErrInvalidValue(`"value" is required for "add" and "replace"`)
	}
	value, err := decode[any](op.Value)
	if err != nil {
		return scimerrors.ErrInvalidValue(`"value" is not valid JSON`)
	}
	m := mutation{value: value, appendMode: appendMode}
	if path.ValueFilter != nil {
		return p.valueWrite(root, path, m)
	}

	attr, err := resolve(p.schemas, path)
	if err != nil {
		return err
	}
	if err := gate(attr, present(root, path)); err != nil {
		return skip(err)
	}
	target, key, err := p.locate(root, path)
	if err != nil {
		return err
	}
	p.store(target, key, attr, m)
	return nil
}

func (p *patcher) remove(root object, op Operation) error {
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
	if err := gate(attr, present(root, path)); err != nil {
		return skip(err)
	}
	if path.SubAttribute == "" {
		root.remove(path.Name)
		return nil
	}
	return removeSub(root, path)
}

func removeSub(root object, path filter.Path) error {
	switch nested := root[root.key(path.Name)].(type) {
	case map[string]any:
		object(nested).remove(path.SubAttribute)
		return nil
	case []any:
		matched, _ := eachMatch(nested, func(map[string]any) bool { return true }, func(member object) error {
			member.remove(path.SubAttribute)
			return nil
		})
		if matched == 0 {
			return scimerrors.ErrNoTarget(`"path" matched no elements`)
		}
		return nil
	}
	return nil
}

func (p *patcher) valueWrite(root object, path filter.Path, m mutation) error {
	parent, err := resolve(p.schemas, base(path))
	if err != nil {
		return err
	}
	if err := gate(parent, present(root, base(path))); err != nil {
		return skip(err)
	}
	pred, err := compile(parent, path.ValueFilter)
	if err != nil {
		return err
	}
	writeOne, err := p.memberWriter(path, parent, m)
	if err != nil {
		return err
	}
	key := root.key(path.Name)
	elements, _ := root[key].([]any)
	matched, err := eachMatch(elements, pred, writeOne)
	if err != nil {
		return err
	}
	if matched == 0 {
		return scimerrors.ErrNoTarget(`"path" filter matched no elements`)
	}
	root[key] = elements
	return nil
}

func (p *patcher) memberWriter(path filter.Path, parent *core.Attribute, m mutation) (func(object) error, error) {
	if path.SubAttribute == "" {
		values, ok := m.value.(map[string]any)
		if !ok {
			return nil, scimerrors.ErrInvalidValue(`"value" must be an object when "path" has no sub-attribute`)
		}
		return func(member object) error {
			return p.mergeAll(member, values, parent, m.appendMode)
		}, nil
	}
	attr, err := resolve(p.schemas, path)
	if err != nil {
		return nil, err
	}
	return func(member object) error {
		if err := gate(attr, member.has(path.SubAttribute)); err != nil {
			return err
		}
		p.store(member, path.SubAttribute, attr, m)
		return nil
	}, nil
}

func (p *patcher) valueRemove(root object, path filter.Path) error {
	parent, err := resolve(p.schemas, base(path))
	if err != nil {
		return err
	}
	if err := gate(parent, present(root, base(path))); err != nil {
		return skip(err)
	}
	pred, err := compile(parent, path.ValueFilter)
	if err != nil {
		return err
	}
	key := root.key(path.Name)
	elements, _ := root[key].([]any)
	if path.SubAttribute != "" {
		return p.clearSub(root, path, elements, pred)
	}
	return dropMembers(root, key, elements, pred)
}

func (p *patcher) clearSub(root object, path filter.Path, elements []any, pred predicate) error {
	attr, err := resolve(p.schemas, path)
	if err != nil {
		return err
	}
	if err := gate(attr, present(root, path)); err != nil {
		return skip(err)
	}
	matched, _ := eachMatch(elements, pred, func(member object) error {
		member.remove(path.SubAttribute)
		return nil
	})
	if matched == 0 {
		return scimerrors.ErrNoTarget(`"path" filter matched no elements`)
	}
	return nil
}

func dropMembers(root object, key string, elements []any, pred predicate) error {
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
	root[key] = kept
	return nil
}

func (p *patcher) mergeAll(target object, values map[string]any, parent *core.Attribute, appendMode bool) error {
	for key, value := range values {
		attr, err := p.attrFor(parent, key)
		if err != nil {
			return err
		}
		if err := gate(attr, target.has(key)); err != nil {
			if errors.Is(err, errSkip) {
				continue
			}
			return err
		}
		p.store(target, key, attr, mutation{value: value, appendMode: appendMode})
	}
	return nil
}

func (p *patcher) store(target object, key string, attr *core.Attribute, m mutation) {
	target.set(key, shaped(m.value, attr.MultiValued), m.appendMode)
}

func (p *patcher) locate(root object, path filter.Path) (object, string, error) {
	if path.SubAttribute == "" {
		return root, path.Name, nil
	}
	nested, err := root.child(path.Name)
	if err != nil {
		return nil, "", err
	}
	return nested, path.SubAttribute, nil
}

func (p *patcher) attrFor(parent *core.Attribute, key string) (*core.Attribute, error) {
	if parent == nil {
		return resolve(p.schemas, topLevelPath(key))
	}
	return subAttr(parent, key), nil
}

func resolve(schemas []*core.Schema, path filter.Path) (*core.Attribute, error) {
	if len(schemas) == 0 {
		return permissiveAttr, nil
	}
	schema := selectSchema(schemas, path.URI)
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

func selectSchema(schemas []*core.Schema, uri string) *core.Schema {
	if uri == "" {
		return schemas[0]
	}
	for _, schema := range schemas {
		if string(schema.ID) == uri {
			return schema
		}
	}
	return nil
}

func subAttr(parent *core.Attribute, name string) *core.Attribute {
	if sub := parent.SubAttribute(name); sub != nil {
		return sub
	}
	return permissiveAttr
}

func topLevelPath(name string) filter.Path {
	return filter.Path{Name: name}
}

func base(path filter.Path) filter.Path {
	path.SubAttribute = ""
	return path
}

func present(root object, path filter.Path) bool {
	if path.SubAttribute == "" {
		return root.has(path.Name)
	}
	if nested, ok := root[root.key(path.Name)].(map[string]any); ok {
		return object(nested).has(path.SubAttribute)
	}
	return false
}

func gate(attr *core.Attribute, present bool) error {
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

func skip(err error) error {
	if errors.Is(err, errSkip) {
		return nil
	}
	return err
}

func eachMatch(elements []any, pred predicate, fn func(object) error) (int, error) {
	matched := 0
	for _, element := range elements {
		member, ok := element.(map[string]any)
		if !ok || !pred(member) {
			continue
		}
		matched++
		if err := fn(object(member)); err != nil {
			if errors.Is(err, errSkip) {
				continue
			}
			return matched, err
		}
	}
	return matched, nil
}
