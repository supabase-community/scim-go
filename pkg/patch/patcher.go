package patch

import (
	"encoding/json/jsontext"
	"errors"
	"strconv"
	"strings"

	"github.com/supabase-community/scim-go/internal/decode"
	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

var permissiveAttr = &core.Attribute{}

type patcher struct {
	schemas core.Schemas
	budget  *budget
	indexes indexCache
}

func (p *patcher) run(root core.Object, ops []Operation) error {
	for _, op := range ops {
		if err := p.apply(root, op); err != nil {
			return err
		}
	}
	return nil
}

func (p *patcher) apply(root core.Object, op Operation) error {
	kind := Op(strings.ToLower(string(op.Op)))
	switch kind {
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
		values, err := decode.Object(op.Value)
		if err != nil {
			return invalidValue(err, `"value" must be an object when "path" is omitted`)
		}
		return p.mergeRoot(root, values, kind)
	}
	path, err := filter.NewPath(op.Path)
	if err != nil {
		return scimerrors.ErrInvalidPath(scimerrors.InvalidPath.Description())
	}
	if len(op.Value) == 0 {
		return scimerrors.ErrInvalidValue(`"value" is required for "add" and "replace"`)
	}
	value, err := decode.Value(op.Value)
	if err != nil {
		return invalidValue(err, `"value" is not valid JSON`)
	}
	return p.writeAt(root, path, kind, value)
}

func (p *patcher) writeAt(root core.Object, path filter.Path, kind Op, value any) error {
	target, err := p.target(root, path)
	if err != nil {
		return err
	}
	return target.write(kind, value)
}

func (p *patcher) remove(root core.Object, op Operation) error {
	if op.Path == "" {
		return scimerrors.ErrNoTarget(`"remove" requires a "path"`)
	}
	if op.hasValue() {
		return scimerrors.ErrInvalidSyntax(`"remove" does not take a "value"`)
	}
	path, err := filter.NewPath(op.Path)
	if err != nil {
		return scimerrors.ErrInvalidPath(scimerrors.InvalidPath.Description())
	}
	target, err := p.target(root, path)
	if err != nil {
		return err
	}
	return target.remove()
}

// RFC 7644 Section 3.5.2.1: if "path" is omitted, the target location is assumed to be the resource itself.
func (p *patcher) mergeRoot(root core.Object, values map[string]any, kind Op) error {
	for key, value := range values {
		if err := p.mergeKey(root, key, value, kind); err != nil {
			return err
		}
	}
	return nil
}

func (p *patcher) mergeKey(root core.Object, key string, value any, kind Op) error {
	if schema := p.schemas.Lookup(core.SchemaURI(key)); schema != nil {
		return p.mergeSchema(root, schema, value, kind)
	}
	path := p.keyPath(key)
	if strings.EqualFold(key, "schemas") || p.repeatsReadOnly(root, path, value) {
		return nil
	}
	return p.writeAt(root, path, kind, value)
}

// RFC 7643 Section 7: readOnly means "The attribute SHALL NOT be modified."
func (p *patcher) repeatsReadOnly(root core.Object, path filter.Path, incoming any) bool {
	attr, err := resolve(p.schemas, path)
	return err == nil && attr.Mutability == core.MutabilityReadOnly && root.Has(path.Name) && value.Equal(attr, root.Get(path.Name), incoming)
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

func (p *patcher) target(root core.Object, path filter.Path) (*target, error) {
	parent, err := resolve(p.schemas, base(path))
	if err != nil {
		return nil, err
	}
	if err := gate(parent); err != nil {
		return nil, err
	}
	t := &target{root: root, extension: p.extension(path), parent: parent, path: path, budget: p.budget, indexes: p.indexes}
	if path.ValueFilter != nil {
		if t.filter.match, t.filter.clauses, err = compile(parent, path.ValueFilter); err != nil {
			return nil, err
		}
	}
	if t.attr, err = resolve(p.schemas, path); err != nil {
		return nil, err
	}
	return t, gate(t.attr)
}

// RFC 7643 Section 3.3: extension attributes live in an object keyed by the extension URN.
func (p *patcher) extension(path filter.Path) string {
	if path.URI == "" {
		return ""
	}
	if schema := p.schemas.Lookup(core.SchemaURI(path.URI)); p.schemas.IsExtension(schema) {
		return string(schema.ID)
	}
	return ""
}

func resolve(schemas core.Schemas, path filter.Path) (*core.Attribute, error) {
	if len(schemas) == 0 {
		return permissiveAttr, nil
	}
	attr, ok := schemas.Resolve(core.SchemaURI(path.URI), path.Name, path.SubAttribute)
	if !ok {
		return nil, scimerrors.ErrInvalidPath(scimerrors.InvalidPath.Description())
	}
	return attr, nil
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

func invalidValue(err error, detail string) error {
	if errors.Is(err, jsontext.ErrDuplicateName) {
		return scimerrors.ErrInvalidValue(`"value" has a repeated attribute name`)
	}
	return scimerrors.ErrInvalidValue(detail)
}
