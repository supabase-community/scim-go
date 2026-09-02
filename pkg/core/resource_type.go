package core

// SchemaExtension is a schema that extends a resource type, per RFC 7643, Section 6.
type SchemaExtension struct {
	Schema   SchemaURI `json:"schema"`
	Required bool      `json:"required"`
}

// ResourceType is the resource type metadata defined in RFC 7643, Section 6.
type ResourceType struct {
	Schemas          []SchemaURI       `json:"schemas,omitempty"`
	ID               ResourceTypeName  `json:"id,omitempty"`
	Name             ResourceTypeName  `json:"name"`
	Description      string            `json:"description,omitempty"`
	Endpoint         string            `json:"endpoint"`
	Schema           SchemaURI         `json:"schema,omitempty"`
	SchemaExtensions []SchemaExtension `json:"schemaExtensions,omitempty"`
	Meta             Meta              `json:"meta,omitzero"`
}

func (r *ResourceType) Extend(extensions ...SchemaExtension) *ResourceType {
	r.SchemaExtensions = append(r.SchemaExtensions, extensions...)
	return r
}

func (r *ResourceType) ResourceID() string {
	return string(r.ID)
}
