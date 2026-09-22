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
	schemaAttribute := core.NewAttribute("userName", core.TypeString).AsCaseExact()
	attrPath, err := filter.NewAttrPath("userName")

	require.NoError(t, err)
	attribute := protocol.NewAttribute(schemaAttribute, attrPath)

	assert.Equal(t, "username", attribute.Key())
	assert.Equal(t, core.TypeString, attribute.Type)
	assert.True(t, attribute.CaseExact)
}
