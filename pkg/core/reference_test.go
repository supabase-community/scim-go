package core_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/supabase-community/scim-go/pkg/core"
)

func TestSchemasReferenceAttributes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		schemas core.Schemas
		want    map[string]core.ReferenceAttribute
	}{
		{
			name: "a User derives groups from Group members and points at a manager",
			schemas: core.Schemas{
				core.NewSchema(core.SchemaUser).With(core.UserAttributes()...),
				core.NewSchema(core.SchemaEnterpriseUser).With(core.EnterpriseUserAttributes()...),
			},
			want: map[string]core.ReferenceAttribute{
				"groups":  {Targets: []core.ReferenceType{"User", "Group"}},
				"manager": {Targets: []core.ReferenceType{"User"}},
			},
		},
		{
			name:    "a Group stores its members",
			schemas: core.Schemas{core.NewSchema(core.SchemaGroup).With(core.GroupAttributes()...)},
			want: map[string]core.ReferenceAttribute{
				"members": {Targets: []core.ReferenceType{"User", "Group"}},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := map[string]core.ReferenceAttribute{}
			for _, reference := range tc.schemas.ReferenceAttributes() {
				name := reference.Name
				reference.Attribute = nil
				got[name] = reference
			}

			assert.Equal(t, tc.want, got)
		})
	}
}

func TestSchemasReferenceAttributesSkipExternalTargets(t *testing.T) {
	attribute := core.NewAttribute("site", core.TypeComplex).With(
		core.NewAttribute("$ref", core.TypeReference).Referencing(core.ReferenceExternal, core.ReferenceURI),
	)
	schemas := core.Schemas{core.NewSchema(core.SchemaUser).With(attribute)}

	assert.Empty(t, schemas.ReferenceAttributes())
}
