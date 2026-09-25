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
	budget  *budget
}

func (p *patcher) run(root core.Object, ops []Operation) error {
	for _, op := range ops {
		err := p.apply(root, op)
		if p.budget.exceeded() {
			return scimerrors.ErrTooLarge("the value filters of the request check more than " + strconv.Itoa(p.budget.max) + " clauses")
		}
		if err != nil {
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
	path, err := filter.NewPath(op.Path)
	if err != nil {
		return scimerrors.ErrInvalidPath(err.Error())
	}
	target, err := p.target(root, path)
	if err != nil {
		return err
	}
	return target.remove()
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

func (p *patcher) target(root core.Object, path filter.Path) (*target, error) {
	parent, err := resolve(p.schemas, base(path))
	if err != nil {
		return nil, err
	}
	if err := gate(parent); err != nil {
		return nil, err
	}
	t := &target{root: root, extension: p.extension(path), parent: parent, path: path}
	if path.ValueFilter != nil {
		if t.match, err = compile(parent, path.ValueFilter, p.budget); err != nil {
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
