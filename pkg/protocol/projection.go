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
	schemas  []*core.Schema
	included names
	excluded names
}

// RFC 7644 Section 3.9: "attributes" and "excludedAttributes" are mutually exclusive.
func ParseProjection(values url.Values, schemas []*core.Schema) (Projection, error) {
	attributes, excluded, err := parseAttributeParams(values)
	if err != nil {
		return Projection{}, err
	}
	return newProjection(schemas, attributes, excluded)
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

func newProjection(schemas []*core.Schema, attributes, excluded []string) (Projection, error) {
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

type projected struct {
	projection Projection
	resource   any
}

func (v projected) MarshalJSON() ([]byte, error) {
	document, err := toDocument(v.resource)
	if err != nil {
		return nil, err
	}
	out := map[string]any{}
	for key, value := range document {
		if key == "schemas" {
			out[key] = value
		} else if projected, ok := v.projection.project(key, value); ok {
			out[key] = projected
		}
	}
	return json.Marshal(out)
}

// names holds fully qualified, lowercase attribute paths: "<schema uri>:<name>[.<sub-name>]",
// or a bare lowercase schema URI when a whole schema/extension was selected.
type names []string

func qualify(schemas []*core.Schema, list []string) (names, error) {
	out := make(names, 0, len(list))
	for _, raw := range list {
		if schema := schemaNamed(schemas, raw); schema != nil {
			out = append(out, strings.ToLower(string(schema.ID)))
			continue
		}
		path, err := filter.NewAttrPath(raw)
		if err != nil {
			return nil, invalidName(raw)
		}
		uri := path.URI
		if uri == "" {
			uri = string(schemas[0].ID)
		}
		qualified := qualifiedKey(core.SchemaURI(uri), path.Name)
		if path.SubAttribute != "" {
			qualified += "." + strings.ToLower(path.SubAttribute)
		}
		out = append(out, qualified)
	}
	return out, nil
}

func qualifiedKey(uri core.SchemaURI, name string) string {
	return strings.ToLower(string(uri)) + ":" + strings.ToLower(name)
}

func schemaNamed(schemas []*core.Schema, name string) *core.Schema {
	for _, schema := range schemas {
		if strings.EqualFold(string(schema.ID), name) {
			return schema
		}
	}
	return nil
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

func (p Projection) project(key string, value any) (any, bool) {
	if attribute, ok := p.schemas[0].Resolve(key); ok {
		return p.value(attribute, qualifiedKey(p.schemas[0].ID, attribute.Name), value)
	}
	if extension := schemaNamed(p.schemas[1:], key); extension != nil {
		return p.extension(extension, value)
	}
	return nil, false
}

func (p Projection) extension(schema *core.Schema, value any) (any, bool) {
	object, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	out := map[string]any{}
	for key, item := range object {
		attribute := schema.Attributes.Lookup(key)
		if attribute == nil {
			continue
		}
		if projected, ok := p.value(attribute, qualifiedKey(schema.ID, attribute.Name), item); ok {
			out[key] = projected
		}
	}
	return out, len(out) > 0
}

// RFC 7643 Section 7: "returned" decides whether an attribute can appear in a response.
func (p Projection) returns(attribute *core.Attribute, name string) bool {
	switch {
	case attribute.Returned == core.ReturnedNever:
		return false
	case attribute.Returned == core.ReturnedAlways:
		return true
	case attribute.Returned == core.ReturnedRequest:
		return slices.Contains(p.included, name) || p.included.within(name)
	case len(p.included) > 0:
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
		object := p.object(attribute, name, v)
		return object, len(object) > 0
	case []any:
		elements := make([]any, 0, len(v))
		for _, element := range v {
			if object, ok := element.(map[string]any); ok {
				element = p.object(attribute, name, object)
			}
			elements = append(elements, element)
		}
		return elements, true
	}
	return value, true
}

func (p Projection) object(attribute *core.Attribute, parentName string, object map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range object {
		sub := attribute.SubAttribute(key)
		if sub == nil {
			continue
		}
		if projected, ok := p.value(sub, parentName+"."+strings.ToLower(sub.Name), value); ok {
			out[key] = projected
		}
	}
	return out
}
