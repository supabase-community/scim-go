package core

import "encoding/json"

// A multi-valued attribute with the default sub-attributes of RFC 7643, Section 2.4.
type MultiValuedAttribute struct {
	Value   string `json:"value,omitempty"`
	Display string `json:"display,omitempty"`
	Type    string `json:"type,omitempty"`
	Primary *bool  `json:"primary,omitempty"`
	Ref     string `json:"$ref,omitempty"`
}

type (
	Email           = MultiValuedAttribute
	PhoneNumber     = MultiValuedAttribute
	IM              = MultiValuedAttribute
	Photo           = MultiValuedAttribute
	Entitlement     = MultiValuedAttribute
	Role            = MultiValuedAttribute
	X509Certificate = MultiValuedAttribute
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
	Primary       bool   `json:"primary,omitempty"`
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

// Manager is the user's manager in the enterprise extension, per RFC 7643, Section 4.3.
type Manager struct {
	Value       string `json:"value,omitempty"`
	Ref         string `json:"$ref,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
}

// EnterpriseUser is the enterprise user schema extension of RFC 7643, Section 4.3.
type EnterpriseUser struct {
	EmployeeNumber string   `json:"employeeNumber,omitempty"`
	CostCenter     string   `json:"costCenter,omitempty"`
	Organization   string   `json:"organization,omitempty"`
	Division       string   `json:"division,omitempty"`
	Department     string   `json:"department,omitempty"`
	Manager        *Manager `json:"manager,omitempty"`
}

// User is the core User resource defined in RFC 7643, Section 4.1.
type User struct {
	CommonAttributes
	ExternalID        string            `json:"externalId,omitempty"`
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

// MarshalJSON omits password, which is writeOnly per RFC 7643, Section 7.
func (u User) MarshalJSON() ([]byte, error) {
	type alias User
	clone := alias(u)
	clone.Password = ""
	return json.Marshal(clone)
}

func (u *User) ResourceID() string {
	return u.ID
}

func (u *User) Location() string {
	return u.Meta.Location
}
