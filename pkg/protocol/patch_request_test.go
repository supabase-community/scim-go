package protocol

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
)

func TestApplyTopLevel(t *testing.T) {
	item := map[string]any{"userName": "old"}
	patch := request(
		operation(PatchOpReplace, "userName", `"new"`),
		operation(PatchOpAdd, "displayName", `"Babs"`),
	)

	require.NoError(t, patch.Apply(item, nil))

	assert.Equal(t, "new", item["userName"])
	assert.Equal(t, "Babs", item["displayName"])
}

func TestApplyTopLevelUser(t *testing.T) {
	item := &core.User{UserName: "old"}
	patch := request(
		operation(PatchOpReplace, "userName", `"new"`),
		operation(PatchOpAdd, "displayName", `"Babs"`),
	)

	require.NoError(t, patch.Apply(item, nil))

	assert.Equal(t, "new", item.UserName)
	assert.Equal(t, "Babs", item.DisplayName)
}

func TestApplyTopLevelGroup(t *testing.T) {
	item := &core.Group{DisplayName: "Tour Guides"}
	patch := request(operation(PatchOpAdd, "displayName", `"Example"`))

	require.NoError(t, patch.Apply(item, nil))
	assert.Equal(t, "Example", item.DisplayName)
}

func TestApplyNoPathMerge(t *testing.T) {
	item := map[string]any{}
	patch := request(PatchOperation{
		Op:    PatchOpAdd,
		Value: json.RawMessage(`{"nickName":"Babs","title":"Eng"}`),
	})

	require.NoError(t, patch.Apply(item, nil))

	assert.Equal(t, "Babs", item["nickName"])
	assert.Equal(t, "Eng", item["title"])
}

func TestApplyAddAppendsAndIsCaseInsensitive(t *testing.T) {
	item := map[string]any{"emails": []any{map[string]any{"value": "a@b.com"}}}
	patch := request(operation(PatchOp("Add"), "emails", `[{"value":"c@d.com"}]`))

	require.NoError(t, patch.Apply(item, nil))

	assert.Len(t, item["emails"], 2)
}

func TestApplyAddSingleValueAppendsToArray(t *testing.T) {
	item := map[string]any{"emails": []any{map[string]any{"value": "a@b.com"}}}
	patch := request(operation(PatchOpAdd, "emails", `{"value":"c@d.com"}`))

	require.NoError(t, patch.Apply(item, nil))

	emails := item["emails"].([]any)
	require.Len(t, emails, 2)
	assert.Equal(t, "a@b.com", emails[0].(map[string]any)["value"])
	assert.Equal(t, "c@d.com", emails[1].(map[string]any)["value"])
}

func TestApplyMissingValueRejected(t *testing.T) {
	patch := request(PatchOperation{Op: PatchOpReplace, Path: "userName"})

	var err *Error
	require.ErrorAs(t, patch.Apply(map[string]any{}, nil), &err)

	assert.Equal(t, ScimTypeInvalidValue, err.ScimType)
}

func TestApplySubAttribute(t *testing.T) {
	item := map[string]any{"name": map[string]any{"familyName": "Old"}}
	patch := request(operation(PatchOpReplace, "name.familyName", `"Jensen"`))

	require.NoError(t, patch.Apply(item, nil))

	assert.Equal(t, "Jensen", item["name"].(map[string]any)["familyName"])
}

func TestApplyRemoveAttribute(t *testing.T) {
	item := map[string]any{"nickName": "Babs"}

	patch := request(operation(PatchOpRemove, "nickName", ""))
	require.NoError(t, patch.Apply(item, nil))

	_, ok := item["nickName"]
	assert.False(t, ok)
}

func TestRemoveWithoutPathIsNoTarget(t *testing.T) {
	patch := request(PatchOperation{Op: PatchOpRemove})

	var err *Error
	require.ErrorAs(t, patch.Apply(map[string]any{}, nil), &err)

	assert.Equal(t, ScimTypeNoTarget, err.ScimType)
}

func TestApplyValuePathReplaceSub(t *testing.T) {
	item := map[string]any{"emails": []any{
		map[string]any{"type": "work", "value": "old@x"},
		map[string]any{"type": "home", "value": "h@x"},
	}}

	patch := request(operation(PatchOpReplace, `emails[type eq "work"].value`, `"new@x"`))
	require.NoError(t, patch.Apply(item, nil))

	emails := item["emails"].([]any)
	assert.Equal(t, "new@x", emails[0].(map[string]any)["value"])
	assert.Equal(t, "h@x", emails[1].(map[string]any)["value"])
}

func TestApplyValuePathRemoveElement(t *testing.T) {
	item := map[string]any{"emails": []any{
		map[string]any{"type": "work", "value": "w@x"},
		map[string]any{"type": "home", "value": "h@x"},
	}}

	patch := request(operation(PatchOpRemove, `emails[type eq "home"]`, ""))
	require.NoError(t, patch.Apply(item, nil))

	assert.Len(t, item["emails"].([]any), 1)
}

func TestApplyValuePathRemoveSubAttribute(t *testing.T) {
	item := map[string]any{"emails": []any{
		map[string]any{"type": "work", "value": "w@x"},
		map[string]any{"type": "home", "value": "h@x"},
	}}

	patch := request(operation(PatchOpRemove, `emails[type eq "work"].value`, ""))
	require.NoError(t, patch.Apply(item, nil))

	emails := item["emails"].([]any)
	assert.Equal(t, map[string]any{"type": "work"}, emails[0])
	assert.Equal(t, map[string]any{"type": "home", "value": "h@x"}, emails[1])
}

func TestApplyValuePathNoMatchIsNoTarget(t *testing.T) {
	item := map[string]any{"emails": []any{map[string]any{"type": "work"}}}

	patch := request(operation(PatchOpReplace, `emails[type eq "home"].value`, `"x"`))

	var err *Error
	require.ErrorAs(t, patch.Apply(item, nil), &err)

	assert.Equal(t, ScimTypeNoTarget, err.ScimType)
}

func TestReadOnlyIsSkipped(t *testing.T) {
	item := map[string]any{"groups": []any{}}
	patch := request(
		operation(PatchOpReplace, "groups", `[{"value":"g1"}]`),
		operation(PatchOpReplace, "userName", `"bjensen"`),
	)

	require.NoError(t, patch.Apply(item, userSchemas()))

	assert.Empty(t, item["groups"])
	assert.Equal(t, "bjensen", item["userName"])
}

func TestIdIsProtected(t *testing.T) {
	item := map[string]any{"id": "keep"}
	patch := request(operation(PatchOpReplace, "id", `"hacked"`))

	require.NoError(t, patch.Apply(item, userSchemas()))
	assert.Equal(t, "keep", item["id"])
}

func TestImmutableAddWhenAbsentAllowed(t *testing.T) {
	item := map[string]any{}
	patch := request(operation(PatchOpAdd, "employeeNumber", `"E1"`))

	require.NoError(t, patch.Apply(item, userSchemas()))
	assert.Equal(t, "E1", item["employeeNumber"])
}

func TestImmutableReplaceWhenAbsentAllowed(t *testing.T) {
	item := map[string]any{}
	patch := request(operation(PatchOpReplace, "employeeNumber", `"E1"`))

	require.NoError(t, patch.Apply(item, userSchemas()))

	assert.Equal(t, "E1", item["employeeNumber"])
}

func TestApplyValuePathRelationalIsCaseInsensitive(t *testing.T) {
	item := map[string]any{"emails": []any{
		map[string]any{"type": "Zebra", "value": "z@x"},
		map[string]any{"type": "ant", "value": "a@x"},
	}}
	patch := request(operation(PatchOpRemove, `emails[type gt "M"]`, ""))

	require.NoError(t, patch.Apply(item, nil))

	emails := item["emails"].([]any)
	require.Len(t, emails, 1)
	assert.Equal(t, "ant", emails[0].(map[string]any)["type"])
}

func TestImmutableChangeRejected(t *testing.T) {
	item := map[string]any{"employeeNumber": "E1"}
	patch := request(operation(PatchOpReplace, "employeeNumber", `"E2"`))

	var err *Error
	require.ErrorAs(t, patch.Apply(item, userSchemas()), &err)

	assert.Equal(t, ScimTypeMutability, err.ScimType)
}

func TestUnknownAttributeRejected(t *testing.T) {
	patch := request(operation(PatchOpAdd, "nonsense", `"x"`))

	var err *Error
	require.ErrorAs(t, patch.Apply(map[string]any{}, userSchemas()), &err)

	assert.Equal(t, ScimTypeInvalidPath, err.ScimType)
}

func TestUnknownSchemaURIRejected(t *testing.T) {
	patch := request(operation(PatchOpReplace, string(core.SchemaUser)+"Bogus:userName", `"x"`))

	var err *Error
	require.ErrorAs(t, patch.Apply(map[string]any{}, userSchemas()), &err)

	assert.Equal(t, ScimTypeInvalidPath, err.ScimType)
}

func TestSchemaURISelectsSchema(t *testing.T) {
	item := map[string]any{"userName": "old"}
	patch := request(operation(PatchOpReplace, string(core.SchemaUser)+":userName", `"new"`))

	require.NoError(t, patch.Apply(item, userSchemas()))

	assert.Equal(t, "new", item["userName"])
}

func TestApplyIsAtomicOnFailure(t *testing.T) {
	item := map[string]any{"userName": "keep", "employeeNumber": "E1"}
	patch := request(
		operation(PatchOpReplace, "userName", `"changed"`),
		operation(PatchOpReplace, "employeeNumber", `"E2"`),
	)

	var err *Error
	require.ErrorAs(t, patch.Apply(item, userSchemas()), &err)

	assert.Equal(t, ScimTypeMutability, err.ScimType)
	assert.Equal(t, "keep", item["userName"])
	assert.Equal(t, "E1", item["employeeNumber"])
}

func TestInvalidOpRejected(t *testing.T) {
	patch := request(operation(PatchOp("delete"), "userName", `"x"`))

	var err *Error
	require.ErrorAs(t, patch.Apply(map[string]any{}, nil), &err)
	assert.Equal(t, ScimTypeInvalidSyntax, err.ScimType)
}

func TestApplyValuePathAndFilter(t *testing.T) {
	item := map[string]any{"emails": []any{
		map[string]any{"type": "work", "primary": true, "value": "w@x"},
		map[string]any{"type": "work", "primary": false, "value": "w2@x"},
	}}
	patch := request(operation(PatchOpReplace, `emails[type eq "work" and primary eq true].value`, `"new@x"`))

	require.NoError(t, patch.Apply(item, nil))

	emails := item["emails"].([]any)
	assert.Equal(t, "new@x", emails[0].(map[string]any)["value"])
	assert.Equal(t, "w2@x", emails[1].(map[string]any)["value"])
}

func TestApplyValuePathOrFilter(t *testing.T) {
	item := map[string]any{"emails": []any{
		map[string]any{"type": "work", "value": "w@x"},
		map[string]any{"type": "home", "value": "h@x"},
		map[string]any{"type": "other", "value": "o@x"},
	}}
	patch := request(operation(PatchOpRemove, `emails[type eq "work" or type eq "home"]`, ""))

	require.NoError(t, patch.Apply(item, nil))
	emails := item["emails"].([]any)
	require.Len(t, emails, 1)
	assert.Equal(t, "other", emails[0].(map[string]any)["type"])
}

func TestApplyValuePathNotFilter(t *testing.T) {
	item := map[string]any{"emails": []any{
		map[string]any{"type": "work", "value": "w@x"},
		map[string]any{"type": "home", "value": "h@x"},
	}}
	patch := request(operation(PatchOpRemove, `emails[not (type eq "work")]`, ""))

	require.NoError(t, patch.Apply(item, nil))
	emails := item["emails"].([]any)
	require.Len(t, emails, 1)
	assert.Equal(t, "work", emails[0].(map[string]any)["type"])
}

func TestApplyValuePathNumericFilter(t *testing.T) {
	item := map[string]any{"scores": []any{
		map[string]any{"kind": "a", "n": float64(10)},
		map[string]any{"kind": "b", "n": float64(2)},
	}}
	patch := request(operation(PatchOpRemove, `scores[n gt 5]`, ""))

	require.NoError(t, patch.Apply(item, nil))

	scores := item["scores"].([]any)
	require.Len(t, scores, 1)
	assert.Equal(t, "b", scores[0].(map[string]any)["kind"])
}

func TestApplyValuePathPresenceFilter(t *testing.T) {
	item := map[string]any{"emails": []any{
		map[string]any{"type": "work", "value": "w@x"},
		map[string]any{"type": "home"},
	}}
	patch := request(operation(PatchOpRemove, `emails[value pr]`, ""))

	require.NoError(t, patch.Apply(item, nil))
	emails := item["emails"].([]any)
	require.Len(t, emails, 1)
	assert.Equal(t, "home", emails[0].(map[string]any)["type"])
}

func TestApplyTypedUserPreservesUntouchedFields(t *testing.T) {
	user := &core.User{UserName: "old", DisplayName: "keep"}
	patch := request(operation(PatchOpReplace, "userName", `"new"`))

	require.NoError(t, patch.Apply(user, nil))
	assert.Equal(t, "new", user.UserName)
	assert.Equal(t, "keep", user.DisplayName)
}

func TestApplyTypedUserValuePath(t *testing.T) {
	active := true
	user := &core.User{
		UserName: "bjensen",
		Active:   &active,
		Emails:   []core.Email{{Type: "work", Value: "old@x"}},
	}
	patch := request(operation(PatchOpReplace, `emails[type eq "work"].value`, `"new@x"`))

	require.NoError(t, patch.Apply(user, nil))
	require.Len(t, user.Emails, 1)
	assert.Equal(t, "new@x", user.Emails[0].Value)
	require.NotNil(t, user.Active)
	assert.True(t, *user.Active)
}

func TestApplyTypedUserRemoveClearsField(t *testing.T) {
	user := &core.User{UserName: "bjensen", DisplayName: "Babs"}
	patch := request(operation(PatchOpRemove, "displayName", ""))

	require.NoError(t, patch.Apply(user, nil))
	assert.Empty(t, user.DisplayName)
	assert.Equal(t, "bjensen", user.UserName)
}

func request(ops ...PatchOperation) *PatchRequest {
	return &PatchRequest{
		Schemas:    []core.SchemaURI{SchemaPatchOp},
		Operations: ops,
	}
}

func operation(kind PatchOp, path, value string) PatchOperation {
	if value != "" {
		return PatchOperation{
			Op:    kind,
			Path:  path,
			Value: json.RawMessage(value),
		}
	}
	return PatchOperation{
		Op:   kind,
		Path: path,
	}
}

func userSchemas() []*core.Schema {
	return []*core.Schema{
		(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("userName", core.TypeString, ""),
			core.NewAttribute("displayName", core.TypeString, ""),
			core.NewAttribute("employeeNumber", core.TypeString, "").AsImmutable(),
			core.NewAttribute("groups", core.TypeComplex, "").AsMultiValued().AsReadOnly().With(
				core.NewAttribute("value", core.TypeString, ""),
			),
			core.NewAttribute("name", core.TypeComplex, "").With(
				core.NewAttribute("familyName", core.TypeString, ""),
			),
			core.NewAttribute("emails", core.TypeComplex, "").AsMultiValued().With(
				core.NewAttribute("type", core.TypeString, ""),
				core.NewAttribute("value", core.TypeString, ""),
			),
		),
	}
}
