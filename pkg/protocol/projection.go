package protocol

import (
	"bytes"
	"encoding/json"
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
	return projection, nil
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
	s, err := newSelector(p, schemas)
	if err != nil {
		return nil, err
	}
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

func toDocument(resource any) (map[string]any, error) {
	raw, err := json.Marshal(resource)
	if err != nil {
		return nil, scimerrors.ErrInternal("could not encode the resource")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	document := map[string]any{}
	if err := decoder.Decode(&document); err != nil {
		return nil, scimerrors.ErrInternal("could not decode the resource")
	}
	return document, nil
}

type selector struct {
	base      *core.Schema
	schemas   []*core.Schema
	selecting bool
	requested []filter.AttrPath
	excluded  []filter.AttrPath
	wanted    []core.SchemaURI
	unwanted  []core.SchemaURI
}

func newSelector(p Projection, schemas []*core.Schema) (*selector, error) {
	s := &selector{base: schemas[0], schemas: schemas, selecting: len(p.Attributes) > 0}
	var err error
	if s.requested, s.wanted, err = parsePaths(p.Attributes, schemas); err != nil {
		return nil, err
	}
	if s.excluded, s.unwanted, err = parsePaths(p.ExcludedAttributes, schemas); err != nil {
		return nil, err
	}
	return s, nil
}

func parsePaths(names []string, schemas []*core.Schema) ([]filter.AttrPath, []core.SchemaURI, error) {
	var paths []filter.AttrPath
	var whole []core.SchemaURI
	for _, name := range names {
		if schema := schemaNamed(schemas, name); schema != nil {
			whole = append(whole, schema.ID)
			continue
		}
		path, err := filter.NewAttrPath(name)
		if err != nil {
			return nil, nil, scimerrors.ErrInvalidValue(`"` + name + `" is not a valid attribute name`)
		}
		paths = append(paths, path)
	}
	return paths, whole, nil
}

func schemaNamed(schemas []*core.Schema, name string) *core.Schema {
	for _, schema := range schemas {
		if strings.EqualFold(string(schema.ID), name) {
			return schema
		}
	}
	return nil
}

func (s *selector) project(key string, value any) (any, bool) {
	if attribute, ok := s.base.Resolve(key); ok {
		return s.attribute(s.base.ID, attribute, value, false)
	}
	if extension := schemaNamed(s.schemas[1:], key); extension != nil {
		return s.extension(extension, value)
	}
	return nil, false
}

func (s *selector) extension(schema *core.Schema, value any) (any, bool) {
	object, ok := value.(map[string]any)
	if !ok || slices.Contains(s.unwanted, schema.ID) {
		return nil, false
	}
	whole := slices.Contains(s.wanted, schema.ID)
	out := map[string]any{}
	for key, item := range object {
		if attribute := schema.Attributes.Lookup(key); attribute != nil {
			if projected, ok := s.attribute(schema.ID, attribute, item, whole); ok {
				out[key] = projected
			}
		}
	}
	return out, len(out) > 0
}

// RFC 7643 Section 7: "returned" decides whether an attribute can appear in a response.
func (s *selector) attribute(uri core.SchemaURI, attribute *core.Attribute, value any, whole bool) (any, bool) {
	switch {
	case attribute.Returned == core.ReturnedNever:
		return nil, false
	case attribute.Returned == core.ReturnedAlways:
		return value, true
	case s.selecting:
		keepAll := whole || s.names(s.requested, uri, attribute.Name)
		keep := s.subsOf(s.requested, uri, attribute.Name)
		if !keepAll && len(keep) == 0 {
			return nil, false
		}
		return subAttributes(attribute, value, choice{all: keepAll, keep: keep})
	case attribute.Returned == core.ReturnedRequest, s.names(s.excluded, uri, attribute.Name):
		return nil, false
	default:
		return subAttributes(attribute, value, choice{all: true, drop: s.subsOf(s.excluded, uri, attribute.Name)})
	}
}

func (s *selector) names(paths []filter.AttrPath, uri core.SchemaURI, name string) bool {
	return slices.ContainsFunc(paths, func(path filter.AttrPath) bool {
		return path.SubAttribute == "" && s.matches(path, uri, name)
	})
}

func (s *selector) subsOf(paths []filter.AttrPath, uri core.SchemaURI, name string) []string {
	var subs []string
	for _, path := range paths {
		if path.SubAttribute != "" && s.matches(path, uri, name) {
			subs = append(subs, path.SubAttribute)
		}
	}
	return subs
}

// RFC 7644 Section 3.10: an unqualified name belongs to the base schema.
func (s *selector) matches(path filter.AttrPath, uri core.SchemaURI, name string) bool {
	inSchema := path.URI == "" && uri == s.base.ID || strings.EqualFold(path.URI, string(uri))
	return inSchema && strings.EqualFold(path.Name, name)
}

type choice struct {
	all  bool
	keep []string
	drop []string
}

func subAttributes(attribute *core.Attribute, value any, c choice) (any, bool) {
	if len(attribute.SubAttributes) == 0 {
		return value, true
	}
	switch v := value.(type) {
	case map[string]any:
		object := subObject(attribute, v, c)
		return object, len(object) > 0
	case []any:
		elements := make([]any, 0, len(v))
		for _, element := range v {
			if object, ok := element.(map[string]any); ok {
				element = subObject(attribute, object, c)
			}
			elements = append(elements, element)
		}
		return elements, true
	}
	return value, true
}

func subObject(attribute *core.Attribute, object map[string]any, c choice) map[string]any {
	out := map[string]any{}
	for key, value := range object {
		if sub := attribute.SubAttribute(key); sub != nil && c.keeps(sub) {
			out[key] = value
		}
	}
	return out
}

func (c choice) keeps(sub *core.Attribute) bool {
	named := func(names []string) bool {
		return slices.ContainsFunc(names, func(name string) bool { return strings.EqualFold(name, sub.Name) })
	}
	switch {
	case sub.Returned == core.ReturnedNever:
		return false
	case sub.Returned == core.ReturnedAlways:
		return true
	case !c.all || sub.Returned == core.ReturnedRequest:
		return named(c.keep)
	default:
		return !named(c.drop)
	}
}
