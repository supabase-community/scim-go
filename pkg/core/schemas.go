package core

import "strings"

const (
	schemaRoot      = "urn:ietf:params:scim:schemas"
	schemaCore      = schemaRoot + ":core:2.0"
	schemaExtension = schemaRoot + ":extension"

	SchemaEnterpriseUser SchemaURI = schemaExtension + ":enterprise:2.0:User"

	SchemaGroup                 SchemaURI = schemaCore + ":Group"
	SchemaResourceType          SchemaURI = schemaCore + ":ResourceType"
	SchemaSchema                SchemaURI = schemaCore + ":Schema"
	SchemaServiceProviderConfig SchemaURI = schemaCore + ":ServiceProviderConfig"
	SchemaUser                  SchemaURI = schemaCore + ":User"
)

// Schemas is a resource's base schema followed by its extensions, per RFC 7643, Section 6.
type Schemas []*Schema

func (s Schemas) Base() *Schema {
	if len(s) == 0 {
		return nil
	}
	return s[0]
}

func (s Schemas) Extensions() Schemas {
	if len(s) == 0 {
		return nil
	}
	return s[1:]
}

func (s Schemas) IsExtension(schema *Schema) bool {
	return schema != nil && schema != s.Base()
}

// Lookup selects a schema by URI; RFC 7644 Section 3.10: an omitted URI means the base schema and URIs are case insensitive.
func (s Schemas) Lookup(uri SchemaURI) *Schema {
	if uri == "" {
		return s.Base()
	}
	for _, schema := range s {
		if strings.EqualFold(string(schema.ID), string(uri)) {
			return schema
		}
	}
	return nil
}

// Resolve finds the attribute at [uri ":"] name ["." sub], per RFC 7644, Section 3.10.
func (s Schemas) Resolve(uri SchemaURI, name, sub string) (*Attribute, bool) {
	schema := s.Lookup(uri)
	if schema == nil {
		return nil, false
	}
	var attribute *Attribute
	if s.IsExtension(schema) {
		attribute = schema.Attributes.Lookup(name)
	} else {
		attribute, _ = schema.Resolve(name)
	}
	if attribute != nil && sub != "" {
		attribute = attribute.SubAttribute(sub)
	}
	return attribute, attribute != nil
}
