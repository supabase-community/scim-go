package core

// ReferenceAttribute is a complex attribute whose "$ref" points at other SCIM resources, per RFC 7643, Section 2.3.7.
type ReferenceAttribute struct {
	*Attribute
	Targets []ReferenceType
}

// ReferenceAttributes lists the attributes of every schema that reference resources of this server, in schema order.
func (s Schemas) ReferenceAttributes() []ReferenceAttribute {
	references := []ReferenceAttribute{}
	for _, schema := range s {
		for _, attribute := range schema.Attributes {
			if targets := attribute.ReferenceTargets(); len(targets) > 0 {
				references = append(references, ReferenceAttribute{Attribute: attribute, Targets: targets})
			}
		}
	}
	return references
}
