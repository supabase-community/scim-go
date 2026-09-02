package server

import (
	"github.com/supabase-community/go-scim/internal/scim"
	"github.com/supabase-community/go-scim/pkg/core"
)

func NewUserSchema(baseURL string) *core.Schema {
	schema := &core.Schema{
		Schemas: []core.SchemaURI{core.SchemaSchema},
		ID:      core.SchemaUser,
		Name:    "User",
		Meta: core.Meta{
			ResourceType: "User",
			Location:     scim.Join(baseURL, "/Users"),
		},
	}

	return schema.
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
	schema := &core.Schema{
		Schemas: []core.SchemaURI{core.SchemaSchema},
		ID:      core.SchemaGroup,
		Name:    "Group",
		Meta: core.Meta{
			ResourceType: "Group",
			Location:     scim.Join(baseURL, "/Groups"),
		},
	}
	return schema.
		Describe("Group").
		With(
			core.NewAttribute("displayName", core.TypeString, "A human-readable name for the Group.").
				AsRequired(),
			core.NewAttribute("members", core.TypeComplex, "A list of members of the Group.").
				AsMultiValued().
				With(
					core.NewAttribute("value", core.TypeString, "The identifier of a member of this Group.").AsImmutable(),
					core.NewAttribute("$ref", core.TypeReference, "The URI of the User or Group that is a member of this Group.").AsImmutable().Referencing("User", "Group"),
					core.NewAttribute("type", core.TypeString, "A label indicating the type of resource, e.g. 'User' or 'Group'.").AsImmutable().Suggesting("User", "Group"),
					core.NewAttribute("display", core.TypeString, "A human-readable name for the member.").AsImmutable(),
				),
		)
}
