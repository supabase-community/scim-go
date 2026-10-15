package protocol_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

func TestAttribute(t *testing.T) {
	emails := core.NewMultiValuedAttribute("emails")
	definition := emails.SubAttribute("type")
	path, err := filter.NewAttrPath("type")
	require.NoError(t, err)

	attribute := protocol.NewAttribute(nil, definition, path, emails)

	assert.Same(t, definition, attribute.Definition)
	assert.Equal(t, "type", attribute.Path.Key())
	assert.Same(t, emails, attribute.Parent)
}

func TestAttributeLocatesTheValueInTheDocument(t *testing.T) {
	schemas := core.Schemas{
		core.NewSchema(core.SchemaUser).With(core.UserAttributes()...),
		core.NewSchema(core.SchemaEnterpriseUser).With(core.EnterpriseUserAttributes()...),
	}
	enterprise := string(core.SchemaEnterpriseUser)
	for _, tc := range []struct {
		sortBy    string
		root      string
		keys      []string
		extension bool
	}{
		{sortBy: "userName", root: "userName", keys: []string{"userName"}},
		{sortBy: "name.givenName", root: "name", keys: []string{"name", "givenName"}},
		{sortBy: "emails", root: "emails", keys: []string{"emails", "value"}},
		{sortBy: "emails.type", root: "emails", keys: []string{"emails", "type"}},
		{sortBy: enterprise + ":employeeNumber", root: "employeeNumber", keys: []string{enterprise, "employeeNumber"}, extension: true},
		{sortBy: enterprise + ":manager.displayName", root: "manager", keys: []string{enterprise, "manager", "displayName"}, extension: true},
	} {
		t.Run(tc.sortBy, func(t *testing.T) {
			request := &protocol.SearchRequest{SortBy: tc.sortBy}

			attribute, err := request.SortAttribute(schemas)

			require.NoError(t, err)
			assert.Equal(t, tc.root, attribute.Root().Name)
			assert.Equal(t, tc.keys, attribute.Keys())
			assert.Equal(t, tc.extension, attribute.InExtension())
		})
	}
}

func TestAttributeInsideAValueFilterIsRelativeToTheElement(t *testing.T) {
	schemas := core.Schemas{core.NewSchema(core.SchemaUser).With(core.UserAttributes()...)}
	emails, _ := schemas.Resolve("", "emails", "")
	definition := emails.SubAttribute("type")
	path, err := filter.NewAttrPath("type")
	require.NoError(t, err)

	attribute := protocol.NewAttribute(schemas, definition, path, emails)

	assert.Same(t, emails, attribute.Root())
	assert.Equal(t, []string{"type"}, attribute.Keys())
	assert.False(t, attribute.InExtension())
}
