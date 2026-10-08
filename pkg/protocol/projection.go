package protocol

import (
	"context"
	"encoding/json"
	"net/url"
	"slices"
	"strings"

	"github.com/supabase-community/scim-go/internal/decode"
	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// Projection selects and renders the attributes of a resource, per RFC 7644, Section 3.9.
type Projection struct {
	schemas  core.Schemas
	included names
	excluded names
	written  names
}

type projectionKey struct{}

type projected struct {
	projection Projection
	resource   any
}

type names []string

// ParseProjection reads "attributes"/"excludedAttributes"; RFC 7644 Section 3.9: they are mutually exclusive.
func ParseProjection(values url.Values, schemas core.Schemas) (Projection, error) {
	return newProjection(schemas, listParam(values, "attributes"), listParam(values, "excludedAttributes"))
}

func WithProjection(ctx context.Context, projection Projection) context.Context {
	return context.WithValue(ctx, projectionKey{}, projection)
}

func ProjectionFrom(ctx context.Context) Projection {
	projection, _ := ctx.Value(projectionKey{}).(Projection)
	return projection
}

func newProjection(schemas core.Schemas, attributes, excluded []string) (Projection, error) {
	if err := checkAttributes(attributes, excluded); err != nil {
		return Projection{}, err
	}
	included, err := qualify(schemas, attributes)
	if err != nil {
		return Projection{}, err
	}
	excludedNames, err := qualify(schemas, excluded)
	if err != nil {
		return Projection{}, err
	}
	return Projection{schemas: schemas, included: included, excluded: excludedNames}, nil
}

// Returns reports whether any part of an attribute, named as in RFC 7644 Section 3.10, can appear in a response.
func (p Projection) Returns(name string) bool {
	path, err := filter.NewAttrPath(name)
	if err != nil || len(p.schemas) == 0 {
		return true
	}
	uri := core.SchemaURI(path.URI)
	attribute, ok := p.schemas.Resolve(uri, path.Name, "")
	return !ok || p.returns(attribute, qualifiedKey(p.schemas.Lookup(uri).ID, attribute.Name)) || returnsAlways(attribute)
}

// Written marks the attributes a POST or PUT body specified, per RFC 7643, Section 7.
func (p Projection) Written(document core.Object) Projection {
	if !p.tracksWrites() {
		return p
	}
	p.written = p.namesIn(document, p.written)
	return p
}

// Patched marks the attributes PATCH operations specified, per RFC 7643, Section 7.
func (p Projection) Patched(operations []patch.Operation) Projection {
	if !p.tracksWrites() {
		return p
	}
	for _, op := range operations {
		if op.Path == "" {
			document, _ := decode.Object(op.Value)
			p.written = p.namesIn(document, p.written)
		} else if path, err := filter.NewPath(op.Path); err == nil {
			p.written = p.write(p.written, path.String())
		}
	}
	return p
}

func (p Projection) Of(resource any) json.Marshaler {
	return projected{projection: p, resource: resource}
}

func (p Projection) All[T any](resources []T) []json.Marshaler {
	out := make([]json.Marshaler, len(resources))
	for i, resource := range resources {
		out[i] = p.Of(resource)
	}
	return out
}

func (p Projection) fillResourceType(document core.Object) {
	if meta := value.AsObject(document["meta"]); meta != nil && len(p.schemas) > 0 {
		if name, _ := meta["resourceType"].(string); name == "" {
			meta["resourceType"] = p.schemas.Base().Name
		}
	}
}

// RFC 7643 Section 3: "schemas" is required and lists the base schema and each extension present.
func (p Projection) fillSchemas(out core.Object) {
	if list, _ := out["schemas"].([]any); len(list) > 0 || len(p.schemas) == 0 {
		return
	}
	uris := []any{string(p.schemas.Base().ID)}
	for _, extension := range p.schemas.Extensions() {
		if _, ok := out[string(extension.ID)]; ok {
			uris = append(uris, string(extension.ID))
		}
	}
	out["schemas"] = uris
}

func (p Projection) project(key string, item any) (any, bool) {
	if len(p.schemas) == 0 {
		return item, true
	}
	base := p.schemas.Base()
	if attribute, ok := p.schemas.Resolve("", key, ""); ok {
		return p.value(attribute, qualifiedKey(base.ID, attribute.Name), item)
	}
	if extension := p.schemas.Lookup(core.SchemaURI(key)); p.schemas.IsExtension(extension) {
		return p.extension(extension, item)
	}
	return nil, false
}

func (p Projection) extension(schema *core.Schema, item any) (any, bool) {
	object := value.AsObject(item)
	if object == nil {
		return nil, false
	}
	prune(object, func(key string, item any) (any, bool) {
		attribute := schema.Attributes.Lookup(key)
		if attribute == nil {
			return nil, false
		}
		return p.value(attribute, qualifiedKey(schema.ID, attribute.Name), item)
	})
	return map[string]any(object), len(object) > 0
}

// RFC 7643 Section 7: "returned" decides whether an attribute can appear in a response.
func (p Projection) returns(attribute *core.Attribute, name string) bool {
	switch {
	case value.Hidden(nil, attribute):
		return false
	case attribute.Returned == core.ReturnedAlways:
		return true
	case attribute.Returned == core.ReturnedRequest:
		return p.requested(name)
	case p.included != nil:
		return p.included.covers(name) || p.included.within(name)
	default:
		return !p.excluded.covers(name)
	}
}

func (p Projection) requested(name string) bool {
	if p.included != nil {
		return slices.Contains(p.included, name) || p.included.within(name)
	}
	return (p.written.covers(name) || p.written.within(name)) && !p.excluded.covers(name)
}

func (p Projection) value(attribute *core.Attribute, name string, item any) (any, bool) {
	if !p.returns(attribute, name) {
		return always(attribute, item)
	}
	if len(attribute.SubAttributes) == 0 {
		return item, true
	}
	if list, ok := item.([]any); ok {
		return keepEach(list, func(element any) (any, bool) { return p.object(attribute, name, element) })
	}
	return p.object(attribute, name, item)
}

func (p Projection) object(attribute *core.Attribute, parentName string, item any) (any, bool) {
	object := value.AsObject(item)
	if object == nil {
		return item, true
	}
	prune(object, func(key string, item any) (any, bool) {
		sub := attribute.SubAttribute(key)
		if sub == nil {
			return nil, false
		}
		return p.value(sub, parentName+"."+strings.ToLower(sub.Name), item)
	})
	return map[string]any(object), len(object) > 0
}

func (p Projection) tracksWrites() bool {
	return p.included == nil && returnsOnRequest(p.schemas)
}

func (p Projection) namesIn(document core.Object, written names) names {
	for key, item := range document {
		if p.schemas.Lookup(core.SchemaURI(key)) == nil {
			written = p.write(written, key)
			continue
		}
		body := value.AsObject(item)
		for sub := range body {
			written = p.write(written, key+":"+sub)
		}
	}
	return written
}

func (p Projection) write(written names, raw string) names {
	if name, known, err := qualifyName(p.schemas, raw); err == nil && known {
		return append(written, name)
	}
	return written
}

func (v projected) MarshalJSON() ([]byte, error) {
	document, err := core.NewObject(v.resource)
	if err != nil {
		return nil, err
	}
	v.projection.fillResourceType(document)
	prune(document, v.projection.project)
	v.projection.fillSchemas(document)
	return json.Marshal(document)
}

func (n names) covers(name string) bool {
	return slices.ContainsFunc(n, func(e string) bool {
		return name == e || strings.HasPrefix(name, e+":") || strings.HasPrefix(name, e+".")
	})
}

func (n names) within(name string) bool {
	return slices.ContainsFunc(n, func(e string) bool { return strings.HasPrefix(e, name+".") })
}

func qualify(schemas core.Schemas, list []string) (names, error) {
	if len(list) == 0 {
		return nil, nil
	}
	out := make(names, 0, len(list))
	for _, raw := range list {
		name, known, err := qualifyName(schemas, raw)
		if err != nil {
			return nil, err
		}
		if known && !slices.Contains(out, name) {
			out = append(out, name)
		}
	}
	return out, nil
}

func qualifyName(schemas core.Schemas, raw string) (string, bool, error) {
	if schema := schemas.Lookup(core.SchemaURI(raw)); schema != nil {
		return strings.ToLower(string(schema.ID)), true, nil
	}
	path, err := filter.NewAttrPath(raw)
	if err != nil {
		return "", false, invalidName()
	}
	schema := schemas.Lookup(core.SchemaURI(path.URI))
	if schema == nil {
		return "", false, invalidName()
	}
	if _, ok := schemas.Resolve(schema.ID, path.Name, path.SubAttribute); !ok {
		return "", false, nil
	}
	qualified := qualifiedKey(schema.ID, path.Name)
	if path.SubAttribute != "" {
		qualified += "." + strings.ToLower(path.SubAttribute)
	}
	return qualified, true, nil
}

func checkAttributes(attributes, excluded []string) error {
	if len(attributes) > 0 && len(excluded) > 0 {
		return scimerrors.ErrInvalidValue(`"attributes" and "excludedAttributes" are mutually exclusive`)
	}
	return nil
}

func invalidName() error {
	return scimerrors.ErrInvalidValue(scimerrors.InvalidValue.Description())
}

func listParam(values url.Values, name string) []string {
	var list []string

	for _, raw := range values[name] {
		for item := range strings.SplitSeq(raw, ",") {
			if item = strings.TrimSpace(item); item != "" {
				list = append(list, item)
			}
		}
	}
	return list
}

func prune(object core.Object, project func(key string, item any) (any, bool)) {
	for key, item := range object {
		if projected, ok := project(key, item); ok {
			object[key] = projected
		} else {
			delete(object, key)
		}
	}
}

func keepEach(list []any, project func(element any) (any, bool)) ([]any, bool) {
	kept := list[:0]
	for _, element := range list {
		if projected, ok := project(element); ok {
			kept = append(kept, projected)
		}
	}
	return kept, len(kept) > 0
}

// RFC 7643 Section 7: an "always" sub-attribute is returned even when its parent is left out.
func always(attribute *core.Attribute, v any) (any, bool) {
	if !returnsAlways(attribute) {
		return nil, false
	}
	if list, ok := v.([]any); ok {
		return keepEach(list, func(element any) (any, bool) { return alwaysIn(attribute, element) })
	}
	return alwaysIn(attribute, v)
}

func alwaysIn(attribute *core.Attribute, v any) (any, bool) {
	object := value.AsObject(v)
	if object == nil {
		return nil, false
	}
	prune(object, func(key string, item any) (any, bool) {
		sub := attribute.SubAttribute(key)
		return item, sub != nil && sub.Returned == core.ReturnedAlways
	})
	return map[string]any(object), len(object) > 0
}

func returnsAlways(attribute *core.Attribute) bool {
	return !value.Hidden(nil, attribute) && slices.ContainsFunc(attribute.SubAttributes, func(sub *core.Attribute) bool {
		return sub.Returned == core.ReturnedAlways
	})
}

func returnsOnRequest(schemas core.Schemas) bool {
	onRequest := func(attribute *core.Attribute) bool { return attribute.Returned == core.ReturnedRequest }
	return slices.ContainsFunc(schemas, func(schema *core.Schema) bool {
		return slices.ContainsFunc(schema.Attributes, func(attribute *core.Attribute) bool {
			return onRequest(attribute) || slices.ContainsFunc(attribute.SubAttributes, onRequest)
		})
	})
}

func qualifiedKey(uri core.SchemaURI, name string) string {
	return strings.ToLower(string(uri)) + ":" + strings.ToLower(name)
}
