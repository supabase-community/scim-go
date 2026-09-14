package protocol_test

import (
	"encoding/json"
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
