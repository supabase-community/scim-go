package core

// EnterpriseUser is the enterprise user schema extension of RFC 7643, Section 4.3.
type EnterpriseUser struct {
	EmployeeNumber string   `json:"employeeNumber,omitempty"`
	CostCenter     string   `json:"costCenter,omitempty"`
	Organization   string   `json:"organization,omitempty"`
	Division       string   `json:"division,omitempty"`
	Department     string   `json:"department,omitempty"`
	Manager        *Manager `json:"manager,omitempty"`
}

// Manager is the user's manager in the enterprise extension, per RFC 7643, Section 4.3.
type Manager struct {
	Value       string `json:"value,omitempty"`
	Ref         string `json:"$ref,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
}

// EnterpriseUserAttributes returns the attributes of the enterprise User extension, per RFC 7643, Section 4.3.
func EnterpriseUserAttributes() Attributes {
	return Attributes{
		NewAttribute("employeeNumber", TypeString),
		NewAttribute("costCenter", TypeString),
		NewAttribute("organization", TypeString),
		NewAttribute("division", TypeString),
		NewAttribute("department", TypeString),
		NewAttribute("manager", TypeComplex).With(
			NewAttribute("value", TypeString),
			NewAttribute("$ref", TypeReference).Referencing("User"),
			NewAttribute("displayName", TypeString).AsReadOnly(),
		),
	}
}
