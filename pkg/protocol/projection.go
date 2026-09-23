package protocol

import (
	"net/url"
	"slices"
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// Projection selects the attributes of a returned resource, per RFC 7644, Section 3.9.
type Projection struct {
	Attributes         []string
	ExcludedAttributes []string
}

// RFC 7644 Section 3.9: "attributes" and "excludedAttributes" are mutually exclusive.
func ParseProjection(values url.Values) (Projection, error) {
	projection := Projection{
		Attributes:         listParam(values, "attributes"),
		ExcludedAttributes: listParam(values, "excludedAttributes"),
	}
	if len(projection.Attributes) > 0 && len(projection.ExcludedAttributes) > 0 {
		return Projection{}, scimerrors.ErrInvalidValue(`"attributes" and "excludedAttributes" are mutually exclusive`)
	}
	for _, name := range slices.Concat(projection.Attributes, projection.ExcludedAttributes) {
		if _, err := filter.NewAttrPath(name); err != nil {
			return Projection{}, invalidName(name)
		}
	}
	return projection, nil
}

func invalidName(name string) error {
	return scimerrors.ErrInvalidValue(`"` + name + `" is not a valid attribute name`)
}

// Apply keeps the declared attributes of resource that schemas return, where schemas[0] is the base schema.
func (p Projection) Apply(resource any, schemas []*core.Schema) (map[string]any, error) {
	document, err := toDocument(resource)
	if err != nil {
		return nil, err
	}
	if len(schemas) == 0 {
		return map[string]any{}, nil
	}
	included, err := qualify(schemas, p.Attributes)
	if err != nil {
		return nil, err
	}
	excluded, err := qualify(schemas, p.ExcludedAttributes)
	if err != nil {
		return nil, err
	}
	s := &selection{schemas: schemas, included: included, excluded: excluded}
	out := map[string]any{}
	for key, value := range document {
		if key == "schemas" {
			out[key] = value
		} else if projected, ok := s.project(key, value); ok {
			out[key] = projected
		}
	}
	return out, nil
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

type selection struct {
	schemas  []*core.Schema
	included names
	excluded names
}

func (s *selection) project(key string, value any) (any, bool) {
	if attribute, ok := s.schemas[0].Resolve(key); ok {
		return s.value(attribute, qualifiedKey(s.schemas[0].ID, attribute.Name), value)
	}
	if extension := schemaNamed(s.schemas[1:], key); extension != nil {
		return s.extension(extension, value)
	}
	return nil, false
}

func (s *selection) extension(schema *core.Schema, value any) (any, bool) {
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
		if projected, ok := s.value(attribute, qualifiedKey(schema.ID, attribute.Name), item); ok {
			out[key] = projected
		}
	}
	return out, len(out) > 0
}

// RFC 7643 Section 7: "returned" decides whether an attribute can appear in a response.
func (s *selection) returns(attribute *core.Attribute, name string) bool {
	switch {
	case attribute.Returned == core.ReturnedNever:
		return false
	case attribute.Returned == core.ReturnedAlways:
		return true
	case attribute.Returned == core.ReturnedRequest:
		return slices.Contains(s.included, name) || s.included.within(name)
	case len(s.included) > 0:
		return s.included.covers(name) || s.included.within(name)
	default:
		return !s.excluded.covers(name)
	}
}

func (s *selection) value(attribute *core.Attribute, name string, value any) (any, bool) {
	if !s.returns(attribute, name) {
		return nil, false
	}
	if len(attribute.SubAttributes) == 0 {
		return value, true
	}
	switch v := value.(type) {
	case map[string]any:
		object := s.object(attribute, name, v)
		return object, len(object) > 0
	case []any:
		elements := make([]any, 0, len(v))
		for _, element := range v {
			if object, ok := element.(map[string]any); ok {
				element = s.object(attribute, name, object)
			}
			elements = append(elements, element)
		}
		return elements, true
	}
	return value, true
}

func (s *selection) object(attribute *core.Attribute, parentName string, object map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range object {
		sub := attribute.SubAttribute(key)
		if sub == nil {
			continue
		}
		if projected, ok := s.value(sub, parentName+"."+strings.ToLower(sub.Name), value); ok {
			out[key] = projected
		}
	}
	return out
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
