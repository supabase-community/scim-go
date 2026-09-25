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

func TestPatchRequestPatchReturnsProtocolError(t *testing.T) {
	request := &protocol.PatchRequest{
		Operations: []patch.Operation{
			{
				Op:   patch.OpReplace,
				Path: "userName",
			},
		},
	}

	var err *scimerrors.Error
	_, patchErr := request.Patch(map[string]any{}, nil)
	require.ErrorAs(t, patchErr, &err)

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

	// RFC 7644 Section 3.5.2: the body MUST contain "schemas" with the PatchOp URI.
	t.Run("rejects a request without the PatchOp schema", func(t *testing.T) {
		_, err := protocol.DecodePatchRequest(strings.NewReader(`{"Operations": [{"op": "replace", "path": "userName", "value": "new"}]}`))

		require.ErrorIs(t, err, scimerrors.ErrInvalidSyntax(""))
	})

	t.Run("matches the PatchOp schema case-insensitively", func(t *testing.T) {
		_, err := protocol.DecodePatchRequest(strings.NewReader(`{
			"schemas": ["URN:IETF:PARAMS:SCIM:API:MESSAGES:2.0:PATCHOP"],
			"Operations": [{"op": "replace", "path": "userName", "value": "new"}]
		}`))

		require.NoError(t, err)
	})

	// RFC 7644 Section 3.5.2: "Operations" is an array of one or more PATCH operations.
	t.Run("rejects a request without operations", func(t *testing.T) {
		_, err := protocol.DecodePatchRequest(strings.NewReader(`{"schemas": ["urn:ietf:params:scim:api:messages:2.0:PatchOp"], "Operations": []}`))

		require.ErrorIs(t, err, scimerrors.ErrInvalidSyntax(""))
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

func TestPatchRequestPatchTypedUser(t *testing.T) {
	apply := func(t *testing.T, user *core.User, operation patch.Operation) *core.User {
		t.Helper()
		patched, err := (&protocol.PatchRequest{Operations: []patch.Operation{operation}}).Patch(user, nil)
		require.NoError(t, err)
		return patched
	}

	t.Run("replaces a value through a value path", func(t *testing.T) {
		active := true
		user := &core.User{UserName: "bjensen", Active: &active, Emails: []core.Email{{Type: "work", Value: "old@x"}}}

		patched := apply(t, user, patch.Operation{Op: patch.OpReplace, Path: `emails[type eq "work"].value`, Value: json.RawMessage(`"new@x"`)})

		assert.Equal(t, []core.Email{{Type: "work", Value: "new@x"}}, patched.Emails)
		assert.Equal(t, &active, patched.Active)
	})

	t.Run("removes a value", func(t *testing.T) {
		patched := apply(t, &core.User{UserName: "bjensen", DisplayName: "Babs"}, patch.Operation{Op: patch.OpRemove, Path: "displayName"})

		assert.Empty(t, patched.DisplayName)
		assert.Equal(t, "bjensen", patched.UserName)
	})
}

func TestPatchRequestPatchReadOnly(t *testing.T) {
	uri := string(core.SchemaEnterpriseUser)
	schemas := core.Schemas{
		(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("userName", core.TypeString),
			core.NewAttribute("parts", core.TypeComplex).AsMultiValued().With(
				core.NewAttribute("serial", core.TypeString),
				core.NewAttribute("inspector", core.TypeString).AsReadOnly(),
			),
		),
		(&core.Schema{ID: core.SchemaEnterpriseUser}).With(
			core.NewAttribute("department", core.TypeString),
		),
	}
	patched := func(t *testing.T, resource map[string]any, operation patch.Operation) map[string]any {
		t.Helper()
		out, err := (&protocol.PatchRequest{Operations: []patch.Operation{operation}}).Patch(resource, schemas)
		require.NoError(t, err)
		return out
	}

	// RFC 7644 Section 3.5.2.2: if no other values remain after removal, the attribute SHALL be considered unassigned.
	t.Run("drops an extension whose last value is removed", func(t *testing.T) {
		out := patched(t, map[string]any{"userName": "bjensen", uri: map[string]any{"department": "eng"}}, patch.Operation{Op: patch.OpRemove, Path: uri + ":department"})

		assert.Equal(t, map[string]any{"userName": "bjensen"}, out)
	})

	// RFC 7643 Section 7: a readOnly attribute SHALL NOT be modified.
	t.Run("keeps readOnly sub-attributes of elements the operation does not target", func(t *testing.T) {
		out := patched(t, map[string]any{"userName": "bjensen", "parts": []any{map[string]any{"serial": "s-1", "inspector": "qa-bot"}}}, patch.Operation{Op: patch.OpReplace, Path: "userName", Value: json.RawMessage(`"babs"`)})

		assert.Equal(t, []any{map[string]any{"serial": "s-1", "inspector": "qa-bot"}}, out["parts"])
	})
}
