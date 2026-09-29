package core

// commonAttributes are the common attributes of RFC 7643 Section 3.1.
var commonAttributes = Attributes{
	NewAttribute("id", TypeString).AsCaseExact().AsReadOnly().ReturnedAs(ReturnedAlways),
	NewAttribute("externalId", TypeString).AsCaseExact(),
	NewAttribute("meta", TypeComplex).AsReadOnly().With(
		NewAttribute("resourceType", TypeString).AsReadOnly(),
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
