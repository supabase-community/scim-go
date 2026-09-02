package server

import "github.com/supabase-community/go-scim/pkg/core"

// NewUserSchema builds the User schema document, per RFC 7643, Section 4.1.
func NewUserSchema(baseURL string) *core.Schema {
	return core.NewSchema(baseURL, core.KindUser).
		Describe("User Account").
		With(
			core.NewAttribute("userName", core.TypeString, "Unique identifier for the User").
				AsRequired().
				UniqueOn(core.UniquenessServer),
			core.NewAttribute("name", core.TypeComplex, "The components of the user's name.").
				With(
					core.NewAttribute("formatted", core.TypeString, "The name formatted for display."),
					core.NewAttribute("familyName", core.TypeString, "The family name of the User."),
					core.NewAttribute("givenName", core.TypeString, "The given name of the User."),
					core.NewAttribute("middleName", core.TypeString, "The middle name(s) of the User."),
				),
			core.NewAttribute("emails", core.TypeComplex, "Email addresses for the user.").
				AsMultiValued().
				With(
					core.NewAttribute("value", core.TypeString, "An email address for the user."),
					core.NewAttribute("primary", core.TypeBoolean, "The 'primary' email address"),
				),
			core.NewAttribute("active", core.TypeBoolean, ""),
		)
}

// NewGroupSchema builds the Group schema document, per RFC 7643, Section 4.2.
func NewGroupSchema(baseURL string) *core.Schema {
	return core.NewSchema(baseURL, core.KindGroup).
		Describe("Group").
		With(
			core.NewAttribute("displayName", core.TypeString, "A human-readable name for the Group.").
				AsRequired(),
			core.NewAttribute("members", core.TypeComplex, "A list of members of the Group.").
				AsMultiValued().
				With(
					core.NewAttribute("value", core.TypeString, "The identifier of a member of this Group.").
						AsImmutable(),
					core.NewAttribute("$ref", core.TypeReference, "The URI of the User or Group that is a member of this Group.").
						AsImmutable().
						Referencing(core.KindUser.Name.Reference(), core.KindGroup.Name.Reference()),
					core.NewAttribute("type", core.TypeString, "A label indicating the type of resource, e.g. 'User' or 'Group'.").
						AsImmutable().
						Suggesting(string(core.KindUser.Name), string(core.KindGroup.Name)),
					core.NewAttribute("display", core.TypeString, "A human-readable name for the member.").
						AsImmutable(),
				),
		)
}
