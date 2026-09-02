package server

import (
	"github.com/supabase-community/go-scim/internal/scim"
	"github.com/supabase-community/go-scim/pkg/core"
)

func NewUserSchema(baseURL string) *core.Schema {
	return scim.NewSchema(baseURL, scim.KindUser).
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

func NewGroupSchema(baseURL string) *core.Schema {
	return scim.NewSchema(baseURL, scim.KindGroup).
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
						Referencing(scim.KindUser.Name.Reference(), scim.KindGroup.Name.Reference()),
					core.NewAttribute("type", core.TypeString, "A label indicating the type of resource, e.g. 'User' or 'Group'.").
						AsImmutable().
						Suggesting(string(scim.KindUser.Name), string(scim.KindGroup.Name)),
					core.NewAttribute("display", core.TypeString, "A human-readable name for the member.").
						AsImmutable(),
				),
		)
}
