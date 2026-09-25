package core

// Element is one element of a multi-valued attribute, per RFC 7643, Section 2.4.
type Element struct {
	Value   string `json:"value,omitempty"`
	Display string `json:"display,omitempty"`
	Type    string `json:"type,omitempty"`
	Primary *bool  `json:"primary,omitempty"`
	Ref     string `json:"$ref,omitempty"`
}

type (
	Email           = Element
	PhoneNumber     = Element
	IM              = Element
	Photo           = Element
	Entitlement     = Element
	Role            = Element
	X509Certificate = Element
)

// GroupMembership is a group the user belongs to, per RFC 7643, Section 4.1.2. It is readOnly.
type GroupMembership struct {
	Value   string `json:"value,omitempty"`
	Ref     string `json:"$ref,omitempty"`
	Display string `json:"display,omitempty"`
	Type    string `json:"type,omitempty"`
}

// Address is a physical mailing address, per RFC 7643, Section 4.1.2.
type Address struct {
	Formatted     string `json:"formatted,omitempty"`
	StreetAddress string `json:"streetAddress,omitempty"`
	Locality      string `json:"locality,omitempty"`
	Region        string `json:"region,omitempty"`
	PostalCode    string `json:"postalCode,omitempty"`
	Country       string `json:"country,omitempty"`
	Type          string `json:"type,omitempty"`
	Primary       *bool  `json:"primary,omitempty"`
}

// Name holds the components of the user's name, per RFC 7643, Section 4.1.1.
type Name struct {
	Formatted       string `json:"formatted,omitempty"`
	FamilyName      string `json:"familyName,omitempty"`
	GivenName       string `json:"givenName,omitempty"`
	MiddleName      string `json:"middleName,omitempty"`
	HonorificPrefix string `json:"honorificPrefix,omitempty"`
	HonorificSuffix string `json:"honorificSuffix,omitempty"`
}

// User is the core User resource defined in RFC 7643, Section 4.1.
type User struct {
	Base
	UserName          string            `json:"userName"`
	Name              Name              `json:"name,omitzero"`
	DisplayName       string            `json:"displayName,omitempty"`
	NickName          string            `json:"nickName,omitempty"`
	ProfileURL        string            `json:"profileUrl,omitempty"`
	Title             string            `json:"title,omitempty"`
	UserType          string            `json:"userType,omitempty"`
	PreferredLanguage string            `json:"preferredLanguage,omitempty"`
	Locale            string            `json:"locale,omitempty"`
	Timezone          string            `json:"timezone,omitempty"`
	Active            *bool             `json:"active,omitempty"`
	Password          string            `json:"password,omitempty"`
	Emails            []Email           `json:"emails,omitempty"`
	PhoneNumbers      []PhoneNumber     `json:"phoneNumbers,omitempty"`
	IMS               []IM              `json:"ims,omitempty"`
	Photos            []Photo           `json:"photos,omitempty"`
	Addresses         []Address         `json:"addresses,omitempty"`
	Groups            []GroupMembership `json:"groups,omitempty"`
	Entitlements      []Entitlement     `json:"entitlements,omitempty"`
	Roles             []Role            `json:"roles,omitempty"`
	X509Certificates  []X509Certificate `json:"x509Certificates,omitempty"`
	EnterpriseUser    *EnterpriseUser   `json:"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User,omitempty"`
}

// UserAttributes returns the attributes of the User schema, per RFC 7643, Section 4.1.
func UserAttributes() Attributes {
	return Attributes{
		NewAttribute("userName", TypeString).AsRequired().UniqueOn(UniquenessServer),
		NewAttribute("name", TypeComplex).With(
			NewAttribute("formatted", TypeString),
			NewAttribute("familyName", TypeString),
			NewAttribute("givenName", TypeString),
			NewAttribute("middleName", TypeString),
			NewAttribute("honorificPrefix", TypeString),
			NewAttribute("honorificSuffix", TypeString),
		),
		NewAttribute("displayName", TypeString),
		NewAttribute("nickName", TypeString),
		NewAttribute("profileUrl", TypeReference).Referencing(ReferenceExternal),
		NewAttribute("title", TypeString),
		NewAttribute("userType", TypeString),
		NewAttribute("preferredLanguage", TypeString),
		NewAttribute("locale", TypeString),
		NewAttribute("timezone", TypeString),
		NewAttribute("active", TypeBoolean),
		NewAttribute("password", TypeString).AsWriteOnly().ReturnedAs(ReturnedNever),
		elements("emails", NewAttribute("value", TypeString), "work", "home", "other"),
		elements("phoneNumbers", NewAttribute("value", TypeString), "work", "home", "mobile", "fax", "pager", "other"),
		elements("ims", NewAttribute("value", TypeString), "aim", "gtalk", "icq", "xmpp", "msn", "skype", "qq", "yahoo"),
		elements("photos", NewAttribute("value", TypeReference).Referencing(ReferenceExternal), "photo", "thumbnail"),
		NewAttribute("addresses", TypeComplex).AsMultiValued().With(
			NewAttribute("formatted", TypeString),
			NewAttribute("streetAddress", TypeString),
			NewAttribute("locality", TypeString),
			NewAttribute("region", TypeString),
			NewAttribute("postalCode", TypeString),
			NewAttribute("country", TypeString),
			NewAttribute("type", TypeString).Suggesting("work", "home", "other"),
			NewAttribute("primary", TypeBoolean),
		),
		NewAttribute("groups", TypeComplex).AsMultiValued().AsReadOnly().With(
			NewAttribute("value", TypeString).AsReadOnly(),
			NewAttribute("$ref", TypeReference).Referencing("User", "Group").AsReadOnly(),
			NewAttribute("display", TypeString).AsReadOnly(),
			NewAttribute("type", TypeString).Suggesting("direct", "indirect").AsReadOnly(),
		),
		elements("entitlements", NewAttribute("value", TypeString)),
		elements("roles", NewAttribute("value", TypeString)),
		elements("x509Certificates", NewAttribute("value", TypeBinary)),
	}
}
