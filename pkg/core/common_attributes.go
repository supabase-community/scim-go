package core

// commonAttributes are the attributes of every resource, per RFC 7643 Sections 3 and 3.1.
var commonAttributes = Attributes{
	NewAttribute("schemas", TypeString).AsMultiValued().AsReadOnly().ReturnedAs(ReturnedAlways),
	NewAttribute("id", TypeString).AsCaseExact().AsReadOnly().ReturnedAs(ReturnedAlways),
	NewAttribute("externalId", TypeString).AsCaseExact(),
	NewAttribute("meta", TypeComplex).AsReadOnly().With(
		NewAttribute("resourceType", TypeString).AsCaseExact().AsReadOnly(),
		NewAttribute("created", TypeDateTime).AsReadOnly(),
		NewAttribute("lastModified", TypeDateTime).AsReadOnly(),
		NewAttribute("location", TypeReference).AsCaseExact().AsReadOnly(),
		NewAttribute("version", TypeString).AsCaseExact().AsReadOnly(),
	),
}

func CommonAttribute(name string) (*Attribute, bool) {
	attribute := commonAttributes.Lookup(name)
	return attribute, attribute != nil
}
