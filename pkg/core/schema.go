package core

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Schema is the schema definition resource of RFC 7643, Section 7.
type Schema struct {
	Schemas     []SchemaURI      `json:"schemas,omitempty"`
	ID          SchemaURI        `json:"id"`
	Name        ResourceTypeName `json:"name"`
	Description string           `json:"description"`
	Attributes  Attributes       `json:"attributes"`
	Meta        Meta             `json:"meta,omitzero"`
}

func NewSchema(id SchemaURI) *Schema {
	return &Schema{
		Schemas: []SchemaURI{SchemaSchema},
		ID:      id,
		Meta: Meta{
			ResourceType: "Schema",
		},
	}
}

func (s *Schema) WithName(name ResourceTypeName) *Schema {
	s.Name = name
	return s
}

func (s *Schema) WithDescription(description string) *Schema {
	s.Description = description
	return s
}

func (s *Schema) WithLocation(location string) *Schema {
	s.Meta.Location = location
	return s
}

func (s *Schema) With(attributes ...*Attribute) *Schema {
	s.Attributes = attributes
	return s
}

func (s *Schema) ResourceID() string {
	return string(s.ID)
}

func (s *Schema) Resolve(name string) (*Attribute, bool) {
	attribute := commonAttributes.Lookup(name)
	if attribute == nil && s != nil {
		attribute = s.Attributes.Lookup(name)
	}
	return attribute, attribute != nil
}

// Validate against the schema's attributes, per RFC 7643, Section 7.
func (s *Schema) Validate(resource any) error {
	raw, err := json.Marshal(resource)
	if err != nil {
		return err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return err
	}
	return validateRequired(s.Attributes, doc)
}

func validateRequired(attrs Attributes, doc map[string]any) error {
	for _, attr := range attrs {
		value, ok := lookupField(doc, attr.Name)
		if attr.Required && (!ok || isEmptyValue(value)) {
			return fmt.Errorf("scim: %q is required", attr.Name)
		}
		if len(attr.SubAttributes) == 0 || !ok || value == nil {
			continue
		}
		if attr.MultiValued {
			items, ok := value.([]any)
			if !ok {
				continue
			}
			for _, item := range items {
				if m, ok := item.(map[string]any); ok {
					if err := validateRequired(attr.SubAttributes, m); err != nil {
						return err
					}
				}
			}
			continue
		}
		if m, ok := value.(map[string]any); ok {
			if err := validateRequired(attr.SubAttributes, m); err != nil {
				return err
			}
		}
	}
	return nil
}

func lookupField(doc map[string]any, name string) (any, bool) {
	if value, ok := doc[name]; ok {
		return value, true
	}
	for key, value := range doc {
		if strings.EqualFold(key, name) {
			return value, true
		}
	}
	return nil, false
}

func isEmptyValue(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return v == ""
	default:
		return false
	}
}
