package protocol_test

import (
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

func TestProjection(t *testing.T) {
	user := (&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
		core.NewAttribute("userName", core.TypeString),
		core.NewAttribute("password", core.TypeString).AsWriteOnly().ReturnedAs(core.ReturnedNever),
		core.NewAttribute("nickName", core.TypeString).ReturnedAs(core.ReturnedRequest),
		core.NewAttribute("name", core.TypeComplex).With(
			core.NewAttribute("givenName", core.TypeString),
			core.NewAttribute("familyName", core.TypeString),
		),
		core.NewMultiValuedAttribute("emails"),
	)
	enterprise := (&core.Schema{ID: core.SchemaEnterpriseUser, Name: "EnterpriseUser"}).With(
		core.NewAttribute("employeeNumber", core.TypeString),
		core.NewAttribute("department", core.TypeString),
	)
	schemas := []*core.Schema{user, enterprise}
	resource := map[string]any{
		"schemas":     []any{string(core.SchemaUser), string(core.SchemaEnterpriseUser)},
		"id":          "2819c223",
		"meta":        map[string]any{"resourceType": "User", "version": `W/"1"`},
		"userName":    "bjensen",
		"password":    "t1meMa$heen",
		"nickName":    "Babs",
		"displayName": "Babs Jensen",
		"name":        map[string]any{"givenName": "Barbara", "familyName": "Jensen"},
		"emails":      []any{map[string]any{"value": "bjensen@example.com", "type": "work"}},
		string(core.SchemaEnterpriseUser): map[string]any{
			"employeeNumber": "701984",
			"department":     "Tour Operations",
		},
	}
	apply := func(t *testing.T, query string) map[string]any {
		t.Helper()
		values, err := url.ParseQuery(query)
		require.NoError(t, err)
		projection, err := protocol.ParseProjection(values)
		require.NoError(t, err)
		out, err := projection.Apply(resource, schemas)
		require.NoError(t, err)
		return out
	}

	t.Run("returns the default set without parameters", func(t *testing.T) {
		out := apply(t, "")
		assert.ElementsMatch(t, []string{"schemas", "id", "meta", "userName", "name", "emails", string(core.SchemaEnterpriseUser)}, keys(out))
	})

	t.Run("never returns a returned:never attribute", func(t *testing.T) {
		assert.NotContains(t, apply(t, "attributes=password"), "password")
	})

	t.Run("returns a returned:request attribute only when requested", func(t *testing.T) {
		assert.Equal(t, "Babs", apply(t, "attributes=nickName")["nickName"])
		assert.NotContains(t, apply(t, "excludedAttributes=userName"), "nickName")
	})

	t.Run("drops attributes the schema does not declare", func(t *testing.T) {
		assert.NotContains(t, apply(t, ""), "displayName")
	})

	// RFC 7644 Section 3.9: the minimum set plus the requested attributes.
	t.Run("returns only the minimum set and the requested attributes", func(t *testing.T) {
		assert.ElementsMatch(t, []string{"schemas", "id", "userName"}, keys(apply(t, "attributes=userName")))
	})

	t.Run("matches attribute names case-insensitively", func(t *testing.T) {
		assert.Equal(t, "bjensen", apply(t, "attributes=USERNAME")["userName"])
	})

	t.Run("returns only the requested sub-attributes", func(t *testing.T) {
		out := apply(t, "attributes=name.givenName,emails.value")
		assert.Equal(t, map[string]any{"givenName": "Barbara"}, out["name"])
		assert.Equal(t, []any{map[string]any{"value": "bjensen@example.com"}}, out["emails"])
	})

	t.Run("removes excluded attributes from the default set", func(t *testing.T) {
		assert.ElementsMatch(t, []string{"schemas", "id", "userName", "name", string(core.SchemaEnterpriseUser)}, keys(apply(t, "excludedAttributes=emails,meta")))
	})

	t.Run("does not exclude a returned:always attribute", func(t *testing.T) {
		assert.Equal(t, "2819c223", apply(t, "excludedAttributes=id")["id"])
	})

	t.Run("removes excluded sub-attributes", func(t *testing.T) {
		assert.Equal(t, map[string]any{"givenName": "Barbara"}, apply(t, "excludedAttributes=name.familyName")["name"])
	})

	t.Run("selects extension attributes by their fully qualified name", func(t *testing.T) {
		out := apply(t, "attributes="+string(core.SchemaEnterpriseUser)+":employeeNumber")
		assert.Equal(t, map[string]any{"employeeNumber": "701984"}, out[string(core.SchemaEnterpriseUser)])
	})

	t.Run("selects a whole extension by its schema URI", func(t *testing.T) {
		assert.Contains(t, apply(t, "attributes="+string(core.SchemaEnterpriseUser)), string(core.SchemaEnterpriseUser))
		assert.NotContains(t, apply(t, "excludedAttributes="+string(core.SchemaEnterpriseUser)), string(core.SchemaEnterpriseUser))
	})

	t.Run("projects a struct through its JSON representation", func(t *testing.T) {
		out, err := protocol.Projection{Attributes: []string{"userName"}}.Apply(&core.User{UserName: "bjensen", Password: "secret"}, schemas)
		require.NoError(t, err)
		assert.Equal(t, "bjensen", out["userName"])
		assert.NotContains(t, out, "password")
	})

	// RFC 7644 Section 3.9: "attributes" and "excludedAttributes" are mutually exclusive.
	t.Run("rejects both parameters together", func(t *testing.T) {
		_, err := protocol.ParseProjection(url.Values{"attributes": {"userName"}, "excludedAttributes": {"emails"}})
		require.ErrorIs(t, err, scimerrors.ErrInvalidValue(""))
	})

	t.Run("rejects an attribute name that is not valid attribute notation", func(t *testing.T) {
		_, err := protocol.Projection{Attributes: []string{"1bad"}}.Apply(resource, schemas)
		require.ErrorIs(t, err, scimerrors.ErrInvalidValue(""))
	})
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
