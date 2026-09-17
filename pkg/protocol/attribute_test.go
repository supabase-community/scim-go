package protocol_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

func TestAttributeExposesResolvedSchemaAndKey(t *testing.T) {
	schemaAttribute := core.NewAttribute("userName", core.TypeString).AsCaseExact()

	attribute := protocol.Attribute{Attribute: schemaAttribute, Key: "username"}

	assert.Equal(t, "username", attribute.Key)
	assert.Equal(t, core.TypeString, attribute.Type)
	assert.True(t, attribute.CaseExact)
}
