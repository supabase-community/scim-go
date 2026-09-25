package protocol

import (
	"encoding/json"
	"net/url"
	"slices"
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// Projection selects and renders the attributes of a resource, per RFC 7644, Section 3.9.
type Projection struct {
	schemas  core.Schemas
	included names
	excluded names
}

// ParseProjection reads "attributes"/"excludedAttributes"; RFC 7644 Section 3.9: they are mutually exclusive.
func ParseProjection(values url.Values, schemas core.Schemas) (Projection, error) {
	attributes, excluded, err := parseAttributeParams(values)
	if err != nil {
		return Projection{}, err
	}
	return newProjection(schemas, attributes, excluded)
}

func newProjection(schemas core.Schemas, attributes, excluded []string) (Projection, error) {
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

// Of renders resource through the projection when the result is marshaled to JSON.
func (p Projection) Of(resource any) json.Marshaler {
	return projected{projection: p, resource: resource}
}

// All renders each of resources through the projection when the result is marshaled to JSON.
func (p Projection) All[T any](resources []T) []json.Marshaler {
	out := make([]json.Marshaler, len(resources))
	for i, resource := range resources {
		out[i] = p.Of(resource)
	}
	return out
}

func (p Projection) fillResourceType(document map[string]any) {
	if meta, ok := document["meta"].(map[string]any); ok && p.schemas != nil {
		if name, _ := meta["resourceType"].(string); name == "" {
			meta["resourceType"] = p.schemas.Base().Name
		}
	}
}

// RFC 7643 Section 3: "schemas" is required and lists the base schema and each extension present.
func (p Projection) fillSchemas(out map[string]any) {
	if list, _ := out["schemas"].([]any); len(list) > 0 || p.schemas == nil {
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

func (p Projection) project(key string, value any) (any, bool) {
	if p.schemas == nil || key == "schemas" {
		return value, true
	}
	base := p.schemas.Base()
	if attribute, ok := p.schemas.Resolve("", key, ""); ok {
		return p.value(attribute, qualifiedKey(base.ID, attribute.Name), value)
	}
	if extension := p.schemas.Lookup(core.SchemaURI(key)); p.schemas.IsExtension(extension) {
		return p.extension(extension, value)
	}
	return nil, false
}

func (p Projection) extension(schema *core.Schema, value any) (any, bool) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	prune(object, func(key string, item any) (any, bool) {
		attribute := schema.Attributes.Lookup(key)
		if attribute == nil {
			return nil, false
		}
		return p.value(attribute, qualifiedKey(schema.ID, attribute.Name), item)
	})
	return object, len(object) > 0
}

// RFC 7643 Section 7: "returned" decides whether an attribute can appear in a response.
func (p Projection) returns(attribute *core.Attribute, name string) bool {
	switch {
	case hidden(attribute):
		return false
	case attribute.Returned == core.ReturnedAlways:
		return true
	case attribute.Returned == core.ReturnedRequest:
		return slices.Contains(p.included, name) || p.included.within(name)
	case p.included != nil:
		return p.included.covers(name) || p.included.within(name)
	default:
		return !p.excluded.covers(name)
	}
}

func (p Projection) value(attribute *core.Attribute, name string, value any) (any, bool) {
	if !p.returns(attribute, name) {
		return nil, false
	}
	if len(attribute.SubAttributes) == 0 {
		return value, true
	}
	switch v := value.(type) {
	case map[string]any:
		p.object(attribute, name, v)
		return v, len(v) > 0
	case []any:
		for _, element := range v {
			if object, ok := element.(map[string]any); ok {
				p.object(attribute, name, object)
			}
		}
		return v, true
	}
	return value, true
}

func (p Projection) object(attribute *core.Attribute, parentName string, object map[string]any) {
	prune(object, func(key string, value any) (any, bool) {
		sub := attribute.SubAttribute(key)
		if sub == nil {
			return nil, false
		}
		return p.value(sub, parentName+"."+strings.ToLower(sub.Name), value)
	})
}

type projected struct {
	projection Projection
	resource   any
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

// names holds fully qualified, lowercase attribute paths: "<schema uri>:<name>[.<sub-name>]",
// or a bare lowercase schema URI when a whole schema/extension was selected.
type names []string

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
		return "", false, invalidName(raw)
	}
	schema := schemas.Lookup(core.SchemaURI(path.URI))
	if schema == nil {
		return "", false, invalidName(raw)
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

// covers reports whether name is selected: an exact match, or nested under a selected
// schema ("<e>:...") or a selected complex attribute ("<e>....").
func (n names) covers(name string) bool {
	return slices.ContainsFunc(n, func(e string) bool {
		return name == e || strings.HasPrefix(name, e+":") || strings.HasPrefix(name, e+".")
	})
}

// within reports whether a selected entry reaches inside name, so name must be
// traversed even though it is not itself selected.
func (n names) within(name string) bool {
	return slices.ContainsFunc(n, func(e string) bool { return strings.HasPrefix(e, name+".") })
}

func parseAttributeParams(values url.Values) (attributes, excluded []string, err error) {
	attributes = listParam(values, "attributes")
	excluded = listParam(values, "excludedAttributes")
	if len(attributes) > 0 && len(excluded) > 0 {
		return nil, nil, scimerrors.ErrInvalidValue(`"attributes" and "excludedAttributes" are mutually exclusive`)
	}
	for _, name := range slices.Concat(attributes, excluded) {
		if _, err := filter.NewAttrPath(name); err != nil {
			return nil, nil, invalidName(name)
		}
	}
	return attributes, excluded, nil
}

func invalidName(name string) error {
	return scimerrors.ErrInvalidValue(`"` + name + `" is not a valid attribute name`)
}

func listParam(values url.Values, name string) []string {
	var list []string

	for _, value := range values[name] {
		for item := range strings.SplitSeq(value, ",") {
			if item = strings.TrimSpace(item); item != "" {
				list = append(list, item)
			}
		}
	}
	return list
}

// prune keeps each member of object that project returns, replaced by its projected value.
func prune(object map[string]any, project func(key string, value any) (any, bool)) {
	for key, value := range object {
		if projected, ok := project(key, value); ok {
			object[key] = projected
		} else {
			delete(object, key)
		}
	}
}

func qualifiedKey(uri core.SchemaURI, name string) string {
	return strings.ToLower(string(uri)) + ":" + strings.ToLower(name)
}
