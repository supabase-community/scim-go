package store_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/store"
)

const memberID = "0198f3a2-7c1e-7b1a-8d3c-2f5a6b7c8d9e"

func TestReferencesOf(t *testing.T) {
	t.Run("marks Group members as forward", func(t *testing.T) {
		references := store.ReferencesOf(groupSchemas())

		names := make([]string, 0, len(references))
		for _, reference := range references {
			names = append(names, reference.Name)
			require.False(t, reference.Inverse())
		}
		require.Contains(t, names, "members")
	})

	t.Run("marks User groups as inverse", func(t *testing.T) {
		for _, reference := range store.ReferencesOf(userSchemas()) {
			if reference.Name == "groups" {
				require.True(t, reference.Inverse())
				require.NotEmpty(t, reference.Types())
				return
			}
		}
		t.Fatal("groups reference not found")
	})
}

func TestLinkDiff(t *testing.T) {
	add, remove := store.LinkDiff([]string{"a", "b", "c"}, []string{"b", "d", "d"})

	require.Equal(t, []string{"d"}, add)
	require.Equal(t, []string{"a", "c"}, remove)
}

func TestEncode(t *testing.T) {
	group := &core.Group{DisplayName: "Admins", Members: []core.Member{{Value: memberID}}}
	group.ID = "ignored"

	doc, err := store.Encode(group, store.ReferencesOf(groupSchemas()))

	require.NoError(t, err)
	require.Equal(t, "ignored", doc.ID)
	require.JSONEq(t, `{"displayName":"Admins"}`, string(withoutSchemas(t, doc.Body)))
	require.Equal(t, []string{memberID}, doc.Links[0].Targets)
	require.Equal(t, "members", doc.Links[0].Attribute)
}

func TestEncodeSoftTargets(t *testing.T) {
	const self = "0198f3a2-7c1e-7b1a-8d3c-2f5a6b7c8d00"
	for value, want := range map[string][]string{memberID: {memberID}, "jsmith@example.com": nil, self: nil} {
		user := &core.User{UserName: "bjensen", EnterpriseUser: &core.EnterpriseUser{Manager: &core.Manager{Value: value}}}
		user.ID = self

		doc, err := store.Encode(user, store.ReferencesOf(userSchemas()))

		require.NoError(t, err)
		require.Contains(t, string(doc.Body), value)
		for _, link := range doc.Links {
			if link.Attribute == "manager" {
				require.Equal(t, want, link.Targets, value)
			}
		}
	}
}

func TestEncodeRejects(t *testing.T) {
	for _, member := range []string{"", "not-a-uuid"} {
		group := &core.Group{Members: []core.Member{{Value: member}}}

		_, err := store.Encode(group, store.ReferencesOf(groupSchemas()))

		require.Error(t, err, member)
	}
}

func TestRecordInto(t *testing.T) {
	updated := time.Date(2026, 7, 22, 8, 12, 3, 0, time.UTC)
	record := store.Record{
		ID:      "42",
		Body:    store.Embed(json.RawMessage(`{"displayName":"Admins"}`), []store.Relation{{Reference: store.ReferencesOf(groupSchemas())[0], ID: memberID, Type: "User"}}),
		Created: updated,
		Updated: updated,
		Version: "W/\"1\"",
	}

	group, err := record.Into[*core.Group]()

	require.NoError(t, err)
	require.Equal(t, "42", group.ID)
	require.Equal(t, "Admins", group.DisplayName)
	require.Equal(t, memberID, group.Members[0].Value)
	require.Equal(t, "W/\"1\"", group.Meta.Version)
}

func TestRelationType(t *testing.T) {
	var groups store.Reference
	for _, reference := range store.ReferencesOf(userSchemas()) {
		if reference.Name == "groups" {
			groups = reference
		}
	}

	for kind, want := range map[string]string{"": "direct", "indirect": "indirect"} {
		object := store.Nest(core.Object{}, []store.Relation{{Reference: groups, ID: memberID, Type: kind}})

		members, ok := object.Get("groups").([]any)
		require.True(t, ok)
		member, ok := members[0].(core.Object)
		require.True(t, ok)
		require.Equal(t, want, member["type"], kind)
	}
}

func TestEmbedMatchesNest(t *testing.T) {
	groups := userReference(t, "groups")
	members := store.ReferencesOf(groupSchemas())[0]
	for name, test := range map[string]struct {
		body      string
		relations []store.Relation
	}{
		"inverse types": {`{"userName":"bjensen"}`, []store.Relation{{Reference: groups, ID: memberID}, {Reference: groups, ID: "g-\"2\"", Type: "indirect"}}},
		"members":       {`{}`, []store.Relation{{Reference: members, ID: memberID, Type: "User"}, {Reference: members, ID: "caf\u00e9", Type: "Group"}, {Reference: members, ID: "\xff", Type: "Group"}}},
		"spaced body":   {"{\"displayName\":\"a\"} \n", []store.Relation{{Reference: members, ID: memberID, Type: "User"}}},
		"no relations":  {`{"displayName":"a"}`, nil},
	} {
		t.Run(name, func(t *testing.T) {
			var want core.Object
			require.NoError(t, json.Unmarshal([]byte(test.body), &want))
			nested, err := json.Marshal(store.Nest(want, test.relations))
			require.NoError(t, err)

			require.JSONEq(t, string(nested), string(store.Embed(json.RawMessage(test.body), test.relations)))
		})
	}
}

func TestRecordIntoRejectsBadJSON(t *testing.T) {
	for _, record := range []store.Record{
		{Body: json.RawMessage(`{`)},
	} {
		_, err := record.Into[*core.Group]()
		require.Error(t, err)
	}
}

func TestDecode(t *testing.T) {
	groups, err := store.Decode[*core.Group]([]store.Record{{ID: "1", Body: json.RawMessage(`{}`)}})
	require.NoError(t, err)
	require.Equal(t, "1", groups[0].ID)

	_, err = store.Decode[*core.Group]([]store.Record{{Body: json.RawMessage(`{`)}})
	require.Error(t, err)
}

func TestUnlink(t *testing.T) {
	schemas := core.Schemas{core.NewSchema(core.SchemaUser).WithName("User").With(core.UserAttributes()...), core.NewSchema(core.SchemaEnterpriseUser).WithName("EnterpriseUser").With(core.EnterpriseUserAttributes()...)}
	object := core.Object{
		"schemas":                         []any{string(core.SchemaUser), string(core.SchemaEnterpriseUser)},
		string(core.SchemaEnterpriseUser): core.Object{"manager": core.Object{"value": memberID}},
	}

	store.Unlink(schemas, object, "manager")

	require.False(t, object.Has(string(core.SchemaEnterpriseUser)))
	require.Equal(t, []any{string(core.SchemaUser)}, object.Get("schemas"))
}

func userReference(t *testing.T, name string) store.Reference {
	t.Helper()
	for _, reference := range store.ReferencesOf(userSchemas()) {
		if reference.Name == name {
			return reference
		}
	}
	t.Fatal(name + " reference not found")
	return store.Reference{}
}

func withoutSchemas(t *testing.T, body json.RawMessage) json.RawMessage {
	t.Helper()
	var object map[string]any
	require.NoError(t, json.Unmarshal(body, &object))
	delete(object, "schemas")
	out, err := json.Marshal(object)
	require.NoError(t, err)
	return out
}

func userSchemas() core.Schemas {
	return core.Schemas{core.NewSchema(core.SchemaUser).WithName("User").With(core.UserAttributes()...)}
}

func groupSchemas() core.Schemas {
	return core.Schemas{core.NewSchema(core.SchemaGroup).WithName("Group").With(core.GroupAttributes()...)}
}
