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

	attribute := protocol.NewAttribute(definition, path, emails)

	assert.Same(t, definition, attribute.Definition)
	assert.Equal(t, "type", attribute.Path.Key())
	assert.Same(t, emails, attribute.Parent)
}
