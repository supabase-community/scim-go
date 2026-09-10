package core

// Schema is the schema definition resource of RFC 7643, Section 7.
type Schema struct {
	Schemas     []SchemaURI      `json:"schemas,omitempty"`
	ID          SchemaURI        `json:"id"`
	Name        ResourceTypeName `json:"name"`
	Description string           `json:"description"`
	Attributes  Attributes       `json:"attributes"`
	Meta        Meta             `json:"meta,omitzero"`
}

func (s *Schema) Resolve(name string) (*Attribute, bool) {
	attribute := commonAttributes.Lookup(name)
	if attribute == nil && s != nil {
		attribute = s.Attributes.Lookup(name)
	}
	return attribute, attribute != nil
}

func (s *Schema) Describe(description string) *Schema {
	s.Description = description
	return s
}

func (s *Schema) With(attributes ...*Attribute) *Schema {
	s.Attributes = attributes
	return s
}

func (s *Schema) ResourceID() string {
	return string(s.ID)
}
