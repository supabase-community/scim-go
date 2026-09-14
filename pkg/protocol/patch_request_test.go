package protocol_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

func TestPatchRequestApplyDelegates(t *testing.T) {
	item := map[string]any{"userName": "old"}
	request := &protocol.PatchRequest{
		Schemas: []core.SchemaURI{protocol.SchemaPatchOp},
		Operations: []protocol.PatchOperation{
			{Op: protocol.PatchOpReplace, Path: "userName", Value: json.RawMessage(`"new"`)},
		},
	}

	require.NoError(t, request.Apply(item, nil))

	assert.Equal(t, "new", item["userName"])
}

func TestPatchRequestApplyReturnsProtocolError(t *testing.T) {
	request := &protocol.PatchRequest{
		Operations: []protocol.PatchOperation{
			{Op: protocol.PatchOpReplace, Path: "userName"},
		},
	}

	var err *protocol.Error
	require.ErrorAs(t, request.Apply(map[string]any{}, nil), &err)

	assert.Equal(t, protocol.ScimTypeInvalidValue, err.ScimType)
}
