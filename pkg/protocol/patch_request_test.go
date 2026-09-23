package protocol_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

func TestPatchRequestApplyDelegates(t *testing.T) {
	item := map[string]any{"userName": "old"}
	request := &protocol.PatchRequest{
		Schemas: []core.SchemaURI{protocol.SchemaPatchOp},
		Operations: []patch.Operation{
			{
				Op:    patch.OpReplace,
				Path:  "userName",
				Value: json.RawMessage(`"new"`),
			},
		},
	}

	require.NoError(t, request.Apply(item, nil))

	assert.Equal(t, "new", item["userName"])
}

func TestPatchRequestApplyReturnsProtocolError(t *testing.T) {
	request := &protocol.PatchRequest{
		Operations: []patch.Operation{
			{
				Op:   patch.OpReplace,
				Path: "userName",
			},
		},
	}

	var err *scimerrors.Error
	require.ErrorAs(t, request.Apply(map[string]any{}, nil), &err)

	assert.Equal(t, scimerrors.InvalidValue, err.ScimType)
}

func TestDecodePatchRequest(t *testing.T) {
	t.Run("decodes a well-formed request", func(t *testing.T) {
		req, err := protocol.DecodePatchRequest(strings.NewReader(`{
			"schemas": ["urn:ietf:params:scim:api:messages:2.0:PatchOp"],
			"Operations": [{"op": "replace", "path": "userName", "value": "new"}]
		}`))

		require.NoError(t, err)
		require.Len(t, req.Operations, 1)
		assert.Equal(t, "userName", req.Operations[0].Path)
	})

	t.Run("rejects a null body", func(t *testing.T) {
		_, err := protocol.DecodePatchRequest(strings.NewReader("null"))

		require.ErrorIs(t, err, scimerrors.ErrInvalidSyntax(""))
	})

	t.Run("rejects a body that is not valid JSON", func(t *testing.T) {
		_, err := protocol.DecodePatchRequest(strings.NewReader("not json"))

		require.ErrorIs(t, err, scimerrors.ErrInvalidSyntax(""))
	})
}

// RFC 7644 Section 3.5.2: a PATCH changes only the attributes it targets, and leaves the original resource untouched.
func TestPatchRequestPatch(t *testing.T) {
	schemas := []*core.Schema{
		(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("userName", core.TypeString),
			core.NewAttribute("userType", core.TypeString),
		),
	}
	request := &protocol.PatchRequest{
		Operations: []patch.Operation{
			{Op: patch.OpReplace, Path: "userType", Value: json.RawMessage(`"employee"`)},
		},
	}
	resource := &core.User{UserName: "bjensen", Password: "t1meMa$heen"}

	patched, err := request.Patch(resource, schemas)

	require.NoError(t, err)
	assert.Equal(t, "employee", patched.UserType)
	assert.Equal(t, "t1meMa$heen", patched.Password)
	assert.NotSame(t, resource, patched)
	assert.Empty(t, resource.UserType)
}
