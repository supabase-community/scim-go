package core

// Member is one member reference of a Group, per RFC 7643, Section 4.2.
type Member struct {
	Value   string           `json:"value,omitempty"`
	Ref     string           `json:"$ref,omitempty"`
	Type    ResourceTypeName `json:"type,omitempty"`
	Display string           `json:"display,omitempty"`
}

// Group is the core Group resource defined in RFC 7643, Section 4.2.
type Group struct {
	Base
	DisplayName string   `json:"displayName"`
	Members     []Member `json:"members,omitempty"`
}

// GroupAttributes returns the attributes of the Group schema, per RFC 7643, Section 4.2.
func GroupAttributes() Attributes {
	return Attributes{
		NewAttribute("displayName", TypeString).AsRequired(),
		NewAttribute("members", TypeComplex).AsMultiValued().With(
			NewAttribute("value", TypeString).AsImmutable(),
			NewAttribute("$ref", TypeReference).Referencing("User", "Group").AsImmutable(),
			NewAttribute("type", TypeString).Suggesting("User", "Group").AsImmutable(),
			NewAttribute("display", TypeString).AsImmutable(),
		),
	}
}
