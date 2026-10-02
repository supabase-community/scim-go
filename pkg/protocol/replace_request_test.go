package protocol_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

func TestDecodeReplaceRequest(t *testing.T) {
	// RFC 7644 Section 3.1: a SCIM resource is a JSON object.
	t.Run("rejects a body that is not a JSON object", func(t *testing.T) {
		_, err := protocol.DecodeReplaceRequest(strings.NewReader(`["bjensen"]`))

		require.ErrorIs(t, err, scimerrors.ErrInvalidSyntax(""))
	})

	t.Run("rejects an attribute name that repeats in another case", func(t *testing.T) {
		_, err := protocol.DecodeReplaceRequest(strings.NewReader(`{"userName":"bjensen","USERNAME":"babs"}`))

		require.ErrorIs(t, err, scimerrors.ErrInvalidSyntax(""))
	})
}

func TestReplaceRequestReplace(t *testing.T) {
	schemas := core.Schemas{core.NewSchema(core.SchemaUser).With(core.UserAttributes()...)}
	existing := core.Object{"id": "u-1", "userName": "bjensen"}
	req, err := protocol.DecodeReplaceRequest(strings.NewReader(`{"id":"forged","userName":"babs"}`))
	require.NoError(t, err)

	// RFC 7644 Section 3.5.1: readOnly values SHALL be ignored.
	replaced, err := req.Replace[*core.User](existing, schemas)

	require.NoError(t, err)
	assert.Equal(t, "u-1", replaced.ID)
	assert.Equal(t, "babs", replaced.UserName)
	assert.Equal(t, core.Object{"id": "u-1", "userName": "babs"}, req.Attributes)
}
