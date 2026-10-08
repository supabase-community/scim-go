package protocol_test

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
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
		core.NewAttribute("credential", core.TypeComplex).With(
			core.NewAttribute("id", core.TypeString).ReturnedAs(core.ReturnedAlways),
			core.NewAttribute("secret", core.TypeString).ReturnedAs(core.ReturnedNever),
			core.NewAttribute("hint", core.TypeString).ReturnedAs(core.ReturnedRequest),
		),
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
		"credential":  map[string]any{"id": "c-1", "secret": "s3cr3t", "hint": "pet"},
		string(core.SchemaEnterpriseUser): map[string]any{
			"employeeNumber": "701984",
			"department":     "Tour Operations",
		},
	}
	marshal := func(t *testing.T, resource any, projection protocol.Projection) map[string]any {
		t.Helper()
		raw, err := json.Marshal(projection.Of(resource))
		require.NoError(t, err)
		var out map[string]any
		require.NoError(t, json.Unmarshal(raw, &out))
		return out
	}
	apply := func(t *testing.T, query string) map[string]any {
		t.Helper()
		values, err := url.ParseQuery(query)
		require.NoError(t, err)
		projection, err := protocol.ParseProjection(values, schemas)
		require.NoError(t, err)
		return marshal(t, resource, projection)
	}

	t.Run("returns the default set without parameters", func(t *testing.T) {
		out := apply(t, "")
		assert.ElementsMatch(t, []string{"schemas", "id", "meta", "userName", "name", "emails", "credential", string(core.SchemaEnterpriseUser)}, keys(out))
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
		assert.ElementsMatch(t, []string{"schemas", "id", "userName", "credential"}, keys(apply(t, "attributes=userName")))
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
		assert.ElementsMatch(t, []string{"schemas", "id", "userName", "name", "credential", string(core.SchemaEnterpriseUser)}, keys(apply(t, "excludedAttributes=emails,meta,credential")))
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
		projection, err := protocol.ParseProjection(url.Values{"attributes": {"userName"}}, schemas)
		require.NoError(t, err)
		out := marshal(t, &core.User{UserName: "bjensen", Password: "secret"}, projection)
		assert.Equal(t, "bjensen", out["userName"])
		assert.NotContains(t, out, "password")
	})

	t.Run("applies returned to sub-attributes", func(t *testing.T) {
		assert.Equal(t, map[string]any{"id": "c-1"}, apply(t, "")["credential"])
		assert.Equal(t, map[string]any{"id": "c-1", "hint": "pet"}, apply(t, "attributes=credential.hint")["credential"])
		assert.Equal(t, map[string]any{"id": "c-1"}, apply(t, "excludedAttributes=credential.id")["credential"])
	})

	t.Run("drops an extension whose value is not an object", func(t *testing.T) {
		projection, err := protocol.ParseProjection(url.Values{}, schemas)
		require.NoError(t, err)
		out := marshal(t, map[string]any{string(core.SchemaEnterpriseUser): "oops"}, projection)
		assert.Equal(t, map[string]any{"schemas": []any{string(core.SchemaUser)}}, out)
	})

	t.Run("reports a resource that cannot be encoded", func(t *testing.T) {
		projection, err := protocol.ParseProjection(url.Values{}, schemas)
		require.NoError(t, err)
		_, err = json.Marshal(projection.Of(map[string]any{"id": make(chan int)}))
		require.ErrorAs(t, err, new(*json.UnsupportedTypeError))
	})

	// RFC 7644 Section 3.9: "attributes" and "excludedAttributes" are mutually exclusive.
	t.Run("rejects both parameters together", func(t *testing.T) {
		_, err := protocol.ParseProjection(url.Values{"attributes": {"userName"}, "excludedAttributes": {"emails"}}, schemas)
		require.ErrorIs(t, err, scimerrors.ErrInvalidValue(""))
	})

	t.Run("rejects an attribute name that is not valid attribute notation", func(t *testing.T) {
		_, err := protocol.ParseProjection(url.Values{"attributes": {"1bad"}}, schemas)
		require.ErrorIs(t, err, scimerrors.ErrInvalidValue(""))

		_, err = protocol.ParseProjection(url.Values{"excludedAttributes": {"1bad"}}, schemas)
		require.ErrorIs(t, err, scimerrors.ErrInvalidValue(""))
	})
}

func TestZeroProjection(t *testing.T) {
	raw, err := json.Marshal(protocol.Projection{}.Of(map[string]any{"id": "2819c223", "userName": "bjensen"}))

	require.NoError(t, err)
	assert.JSONEq(t, `{"id":"2819c223","userName":"bjensen"}`, string(raw))
}

func TestProjectionWithEmptySchemas(t *testing.T) {
	projection, err := protocol.ParseProjection(url.Values{}, core.Schemas{})
	require.NoError(t, err)

	raw, err := json.Marshal(projection.Of(map[string]any{"userName": "bjensen", "meta": map[string]any{}}))

	require.NoError(t, err)
	assert.JSONEq(t, `{"meta":{},"userName":"bjensen"}`, string(raw))
	assert.True(t, projection.Returns("userName"))
}

func TestProjectionReturns(t *testing.T) {
	schemas := core.Schemas{core.NewSchema(core.SchemaGroup).With(core.GroupAttributes()...)}
	group := map[string]any{"members": []any{map[string]any{"value": "2819c223", "type": "User"}}}
	for query, want := range map[string]bool{
		"":                           true,
		"excludedAttributes=members": false,
		"excludedAttributes=MEMBERS": false,
		"excludedAttributes=urn:ietf:params:scim:schemas:core:2.0:Group:members": false,
		"excludedAttributes=displayName":                                         true,
		"excludedAttributes=members.display":                                     true,
		"attributes=displayName":                                                 false,
		"attributes=members":                                                     true,
		"attributes=members.value":                                               true,
		"attributes=members.type":                                                true,
		"attributes=urn:ietf:params:scim:schemas:core:2.0:Group:Members":         true,
	} {
		values, err := url.ParseQuery(query)
		require.NoError(t, err)
		projection, err := protocol.ParseProjection(values, schemas)
		require.NoError(t, err, query)
		raw, err := json.Marshal(projection.Of(group))
		require.NoError(t, err)

		assert.Equal(t, want, protocol.ProjectionFrom(protocol.WithProjection(context.Background(), projection)).Returns("members"), query)
		assert.Equal(t, want, strings.Contains(string(raw), `"members"`), query)
	}
	assert.True(t, protocol.ProjectionFrom(context.Background()).Returns("members"))
}

// RFC 7643 Section 7: an "always" sub-attribute is returned regardless of the "attributes" parameter.
func TestProjectionReturnsAnAlwaysSubAttribute(t *testing.T) {
	schemas := core.Schemas{core.NewSchema(core.SchemaUser).With(
		core.NewAttribute("userName", core.TypeString),
		core.NewAttribute("keys", core.TypeComplex).AsMultiValued().With(
			core.NewAttribute("id", core.TypeString).ReturnedAs(core.ReturnedAlways),
			core.NewAttribute("label", core.TypeString),
		),
	)}
	user := map[string]any{"userName": "bjensen", "keys": []any{map[string]any{"id": "k-1", "label": "a"}, map[string]any{"label": "b"}}}
	projection, err := protocol.ParseProjection(url.Values{"attributes": {"userName"}}, schemas)
	require.NoError(t, err)
	raw, err := json.Marshal(projection.Of(user))
	require.NoError(t, err)

	assert.True(t, projection.Returns("keys"))
	assert.JSONEq(t, `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"bjensen","keys":[{"id":"k-1"}]}`, string(raw))
}

func TestProjectionReturnsAnExtensionAttribute(t *testing.T) {
	schemas := core.Schemas{
		core.NewSchema(core.SchemaUser).With(core.UserAttributes()...),
		core.NewSchema(core.SchemaEnterpriseUser).With(core.EnterpriseUserAttributes()...),
	}
	manager := string(core.SchemaEnterpriseUser) + ":manager"
	projection, err := protocol.ParseProjection(url.Values{"excludedAttributes": {manager}}, schemas)
	require.NoError(t, err)

	assert.False(t, projection.Returns(manager))
	assert.True(t, projection.Returns(string(core.SchemaEnterpriseUser)+":department"))
	assert.True(t, projection.Returns("bogus"))
}

// RFC 7643 Section 7: a "request" attribute is returned when the client wrote it.
func TestProjectionReturnsAWrittenExtensionRequestAttribute(t *testing.T) {
	schemas := core.Schemas{
		core.NewSchema(core.SchemaUser).With(core.UserAttributes()...),
		core.NewSchema(core.SchemaEnterpriseUser).With(core.NewAttribute("badge", core.TypeString).ReturnedAs(core.ReturnedRequest)),
	}
	document := core.Object{"userName": "bjensen", string(core.SchemaEnterpriseUser): map[string]any{"badge": "b1"}}
	projection, err := protocol.ParseProjection(url.Values{}, schemas)
	require.NoError(t, err)

	written, err := json.Marshal(projection.Written(document).Of(document))
	require.NoError(t, err)
	unwritten, err := json.Marshal(projection.Of(document))
	require.NoError(t, err)

	assert.Contains(t, string(written), `"badge":"b1"`)
	assert.NotContains(t, string(unwritten), `"badge"`)
}

func TestProjectionFillsSchemasAndResourceType(t *testing.T) {
	schemas := []*core.Schema{
		(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(core.NewAttribute("userName", core.TypeString)),
		(&core.Schema{ID: core.SchemaEnterpriseUser, Name: "EnterpriseUser"}).With(core.NewAttribute("department", core.TypeString)),
	}
	projection, err := protocol.ParseProjection(url.Values{}, schemas)
	require.NoError(t, err)

	for name, test := range map[string]struct {
		user     *core.User
		expected string
	}{
		"when empty": {
			&core.User{UserName: "bjensen", EnterpriseUser: &core.EnterpriseUser{Department: "Tour"}},
			`["` + string(core.SchemaUser) + `","` + string(core.SchemaEnterpriseUser) + `"]|User`,
		},
		"but keeps values that are set": {
			&core.User{Schemas: []core.SchemaURI{"urn:x"}, Meta: core.Meta{ResourceType: "Person"}},
			`["urn:x"]|Person`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(projection.Of(test.user))
			require.NoError(t, err)

			var out struct {
				Schemas json.RawMessage `json:"schemas"`
				Meta    core.Meta       `json:"meta"`
			}
			require.NoError(t, json.Unmarshal(raw, &out))
			assert.Equal(t, test.expected, string(out.Schemas)+"|"+string(out.Meta.ResourceType))
		})
	}
}

func TestProjectionAll(t *testing.T) {
	schemas := []*core.Schema{(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(core.NewAttribute("userName", core.TypeString))}
	projection, err := protocol.ParseProjection(url.Values{"attributes": {"userName"}}, schemas)
	require.NoError(t, err)

	resources := []map[string]any{{"userName": "alice", "id": "1"}, {"userName": "bob", "id": "2"}}
	raw, err := json.Marshal(projection.All(resources))

	require.NoError(t, err)
	schema := `"schemas": ["` + string(core.SchemaUser) + `"]`
	assert.JSONEq(t, `[{`+schema+`, "userName": "alice", "id": "1"}, {`+schema+`, "userName": "bob", "id": "2"}]`, string(raw))
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
