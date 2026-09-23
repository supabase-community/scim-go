package protocol_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// RFC 7644 Sections 3.3 and 3.5.1: values provided for readOnly attributes SHALL be ignored.
func TestWritable(t *testing.T) {
	user := (&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
		core.NewAttribute("userName", core.TypeString),
		core.NewAttribute("groups", core.TypeComplex).AsMultiValued().AsReadOnly().With(
			core.NewAttribute("value", core.TypeString),
		),
		core.NewAttribute("badge", core.TypeComplex).With(
			core.NewAttribute("label", core.TypeString),
			core.NewAttribute("issuedBy", core.TypeString).AsReadOnly(),
		),
		core.NewAttribute("keys", core.TypeComplex).AsMultiValued().With(
			core.NewAttribute("value", core.TypeString),
			core.NewAttribute("fingerprint", core.TypeString).AsReadOnly(),
		),
	)
	enterprise := (&core.Schema{ID: core.SchemaEnterpriseUser, Name: "EnterpriseUser"}).With(
		core.NewAttribute("department", core.TypeString),
		core.NewAttribute("employeeNumber", core.TypeString).AsReadOnly(),
	)
	schemas := []*core.Schema{user, enterprise}
	uri := string(core.SchemaEnterpriseUser)
	body := func() map[string]any {
		return map[string]any{
			"schemas":  []any{string(core.SchemaUser)},
			"id":       "client-chosen",
			"meta":     map[string]any{"created": "2000-01-01T00:00:00Z"},
			"userName": "bjensen",
			"nickName": "Babs",
			"groups":   []any{map[string]any{"value": "admins"}},
			"badge":    map[string]any{"label": "B", "issuedBy": "client"},
			"keys":     []any{map[string]any{"value": "k1", "fingerprint": "client"}},
			uri:        map[string]any{"department": "Tour Operations", "employeeNumber": "client"},
		}
	}
	existing := map[string]any{
		"id":       "2819c223",
		"meta":     map[string]any{"created": "2026-07-21T19:41:41Z"},
		"userName": "old",
		"groups":   []any{map[string]any{"value": "staff"}},
		"badge":    map[string]any{"label": "A", "issuedBy": "server"},
		uri:        map[string]any{"employeeNumber": "701984"},
	}

	t.Run("drops readOnly values when there is no existing resource", func(t *testing.T) {
		out, err := protocol.Writable(body(), nil, schemas)
		require.NoError(t, err)
		assert.Equal(t, map[string]any{
			"schemas":  []any{string(core.SchemaUser)},
			"userName": "bjensen",
			"nickName": "Babs",
			"badge":    map[string]any{"label": "B"},
			"keys":     []any{map[string]any{"value": "k1"}},
			uri:        map[string]any{"department": "Tour Operations"},
		}, out)
	})

	t.Run("keeps the existing readOnly values", func(t *testing.T) {
		out, err := protocol.Writable(body(), existing, schemas)
		require.NoError(t, err)
		assert.Equal(t, "2819c223", out["id"])
		assert.Equal(t, existing["meta"], out["meta"])
		assert.Equal(t, "bjensen", out["userName"])
		assert.Equal(t, existing["groups"], out["groups"])
		assert.Equal(t, map[string]any{"label": "B", "issuedBy": "server"}, out["badge"])
		assert.Equal(t, map[string]any{"department": "Tour Operations", "employeeNumber": "701984"}, out[uri])
	})

	t.Run("keeps existing readOnly values the body leaves out", func(t *testing.T) {
		out, err := protocol.Writable(map[string]any{"userName": "bjensen"}, existing, schemas)
		require.NoError(t, err)
		assert.Equal(t, existing["groups"], out["groups"])
		assert.Equal(t, map[string]any{"employeeNumber": "701984"}, out[uri])
		assert.NotContains(t, out, "badge")
	})

	t.Run("leaves an extension that is not an object for decoding to reject", func(t *testing.T) {
		out, err := protocol.Writable(map[string]any{uri: "oops"}, nil, schemas)
		require.NoError(t, err)
		assert.Equal(t, "oops", out[uri])
	})

	t.Run("returns the body without schemas", func(t *testing.T) {
		out, err := protocol.Writable(body(), existing, nil)
		require.NoError(t, err)
		assert.Equal(t, body(), out)
	})

	t.Run("reports an existing resource that cannot be encoded", func(t *testing.T) {
		_, err := protocol.Writable(body(), map[string]any{"id": make(chan int)}, schemas)
		require.ErrorIs(t, err, scimerrors.ErrInternal(""))
	})
}
