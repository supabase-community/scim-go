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
	existing := &core.User{ID: "u-1", UserName: "bjensen"}
	req, err := protocol.DecodeReplaceRequest(strings.NewReader(`{"id":"forged","userName":"babs"}`))
	require.NoError(t, err)

	// RFC 7644 Section 3.5.1: readOnly values SHALL be ignored.
	t.Run("keeps readOnly values of the existing resource", func(t *testing.T) {
		replaced, err := req.Replace(existing, schemas)

		require.NoError(t, err)
		assert.Equal(t, "u-1", replaced.ID)
		assert.Equal(t, "babs", replaced.UserName)
	})

	t.Run("applies to each existing resource it is given", func(t *testing.T) {
		first, err := req.Replace(existing, schemas)
		require.NoError(t, err)
		second, err := req.Replace(&core.User{ID: "u-2", UserName: "bjensen"}, schemas)
		require.NoError(t, err)

		assert.Equal(t, "u-1", first.ID)
		assert.Equal(t, "u-2", second.ID)
		assert.Equal(t, first.UserName, second.UserName)
		assert.Equal(t, core.Object{"id": "forged", "userName": "babs"}, req.Attributes)
	})
}
