package protocol

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
)

func patch(ops ...PatchOperation) *PatchRequest {
	return &PatchRequest{
		Schemas:    []core.SchemaURI{SchemaPatchOp},
		Operations: ops,
	}
}

func op(kind PatchOp, path, value string) PatchOperation {
	o := PatchOperation{Op: kind, Path: path}
	if value != "" {
		o.Value = json.RawMessage(value)
	}
	return o
}

func userSchemas() []*core.Schema {
	schema := &core.Schema{ID: core.SchemaUser, Name: "User"}
	return []*core.Schema{schema.With(
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
	)}
}

func TestApplyTopLevel(t *testing.T) {
	doc := map[string]any{"userName": "old"}
	err := patch(
		op(PatchOpReplace, "userName", `"new"`),
		op(PatchOpAdd, "displayName", `"Babs"`),
	).Apply(doc, nil)
	require.NoError(t, err)
	assert.Equal(t, "new", doc["userName"])
	assert.Equal(t, "Babs", doc["displayName"])
}

func TestApplyNoPathMerge(t *testing.T) {
	doc := map[string]any{}
	err := patch(PatchOperation{
		Op:    PatchOpAdd,
		Value: json.RawMessage(`{"nickName":"Babs","title":"Eng"}`),
	}).Apply(doc, nil)
	require.NoError(t, err)
	assert.Equal(t, "Babs", doc["nickName"])
	assert.Equal(t, "Eng", doc["title"])
}

func TestApplyAddAppendsAndIsCaseInsensitive(t *testing.T) {
	doc := map[string]any{"emails": []any{map[string]any{"value": "a@b.com"}}}
	err := patch(op(PatchOp("Add"), "emails", `[{"value":"c@d.com"}]`)).Apply(doc, nil)
	require.NoError(t, err)
	assert.Len(t, doc["emails"], 2)
}

func TestApplyAddSingleValueAppendsToArray(t *testing.T) {
	doc := map[string]any{"emails": []any{map[string]any{"value": "a@b.com"}}}
	err := patch(op(PatchOpAdd, "emails", `{"value":"c@d.com"}`)).Apply(doc, nil)
	require.NoError(t, err)
	emails := doc["emails"].([]any)
	require.Len(t, emails, 2)
	assert.Equal(t, "a@b.com", emails[0].(map[string]any)["value"])
	assert.Equal(t, "c@d.com", emails[1].(map[string]any)["value"])
}

func TestApplyMissingValueRejected(t *testing.T) {
	err := patch(PatchOperation{Op: PatchOpReplace, Path: "userName"}).Apply(map[string]any{}, nil)
	var scimErr *Error
	require.ErrorAs(t, err, &scimErr)
	assert.Equal(t, ScimTypeInvalidValue, scimErr.ScimType)
}

func TestApplySubAttribute(t *testing.T) {
	doc := map[string]any{"name": map[string]any{"familyName": "Old"}}
	err := patch(op(PatchOpReplace, "name.familyName", `"Jensen"`)).Apply(doc, nil)
	require.NoError(t, err)
	assert.Equal(t, "Jensen", doc["name"].(map[string]any)["familyName"])
}

func TestApplyRemoveAttribute(t *testing.T) {
	doc := map[string]any{"nickName": "Babs"}
	err := patch(op(PatchOpRemove, "nickName", "")).Apply(doc, nil)
	require.NoError(t, err)
	_, ok := doc["nickName"]
	assert.False(t, ok)
}

func TestRemoveWithoutPathIsNoTarget(t *testing.T) {
	err := patch(PatchOperation{Op: PatchOpRemove}).Apply(map[string]any{}, nil)
	var scimErr *Error
	require.ErrorAs(t, err, &scimErr)
	assert.Equal(t, ScimTypeNoTarget, scimErr.ScimType)
}

func TestApplyValuePathReplaceSub(t *testing.T) {
	doc := map[string]any{"emails": []any{
		map[string]any{"type": "work", "value": "old@x"},
		map[string]any{"type": "home", "value": "h@x"},
	}}
	err := patch(op(PatchOpReplace, `emails[type eq "work"].value`, `"new@x"`)).Apply(doc, nil)
	require.NoError(t, err)
	emails := doc["emails"].([]any)
	assert.Equal(t, "new@x", emails[0].(map[string]any)["value"])
	assert.Equal(t, "h@x", emails[1].(map[string]any)["value"])
}

func TestApplyValuePathRemoveElement(t *testing.T) {
	doc := map[string]any{"emails": []any{
		map[string]any{"type": "work", "value": "w@x"},
		map[string]any{"type": "home", "value": "h@x"},
	}}
	err := patch(op(PatchOpRemove, `emails[type eq "home"]`, "")).Apply(doc, nil)
	require.NoError(t, err)
	assert.Len(t, doc["emails"].([]any), 1)
}

func TestApplyValuePathRemoveSubAttribute(t *testing.T) {
	doc := map[string]any{"emails": []any{
		map[string]any{"type": "work", "value": "w@x"},
		map[string]any{"type": "home", "value": "h@x"},
	}}
	err := patch(op(PatchOpRemove, `emails[type eq "work"].value`, "")).Apply(doc, nil)
	require.NoError(t, err)
	emails := doc["emails"].([]any)
	assert.Equal(t, map[string]any{"type": "work"}, emails[0])
	assert.Equal(t, map[string]any{"type": "home", "value": "h@x"}, emails[1])
}

func TestApplyValuePathNoMatchIsNoTarget(t *testing.T) {
	doc := map[string]any{"emails": []any{map[string]any{"type": "work"}}}
	err := patch(op(PatchOpReplace, `emails[type eq "home"].value`, `"x"`)).Apply(doc, nil)
	var scimErr *Error
	require.ErrorAs(t, err, &scimErr)
	assert.Equal(t, ScimTypeNoTarget, scimErr.ScimType)
}

func TestReadOnlyIsSkipped(t *testing.T) {
	doc := map[string]any{"groups": []any{}}
	err := patch(
		op(PatchOpReplace, "groups", `[{"value":"g1"}]`),
		op(PatchOpReplace, "userName", `"bjensen"`),
	).Apply(doc, userSchemas())
	require.NoError(t, err)
	assert.Empty(t, doc["groups"])
	assert.Equal(t, "bjensen", doc["userName"])
}

func TestIdIsProtected(t *testing.T) {
	doc := map[string]any{"id": "keep"}
	err := patch(op(PatchOpReplace, "id", `"hacked"`)).Apply(doc, userSchemas())
	require.NoError(t, err)
	assert.Equal(t, "keep", doc["id"])
}

func TestImmutableAddWhenAbsentAllowed(t *testing.T) {
	doc := map[string]any{}
	err := patch(op(PatchOpAdd, "employeeNumber", `"E1"`)).Apply(doc, userSchemas())
	require.NoError(t, err)
	assert.Equal(t, "E1", doc["employeeNumber"])
}

func TestImmutableReplaceWhenAbsentAllowed(t *testing.T) {
	doc := map[string]any{}
	err := patch(op(PatchOpReplace, "employeeNumber", `"E1"`)).Apply(doc, userSchemas())
	require.NoError(t, err)
	assert.Equal(t, "E1", doc["employeeNumber"])
}

func TestApplyValuePathRelationalIsCaseInsensitive(t *testing.T) {
	doc := map[string]any{"emails": []any{
		map[string]any{"type": "Zebra", "value": "z@x"},
		map[string]any{"type": "ant", "value": "a@x"},
	}}
	err := patch(op(PatchOpRemove, `emails[type gt "M"]`, "")).Apply(doc, nil)
	require.NoError(t, err)
	emails := doc["emails"].([]any)
	require.Len(t, emails, 1)
	assert.Equal(t, "ant", emails[0].(map[string]any)["type"])
}

func TestImmutableChangeRejected(t *testing.T) {
	doc := map[string]any{"employeeNumber": "E1"}
	err := patch(op(PatchOpReplace, "employeeNumber", `"E2"`)).Apply(doc, userSchemas())
	var scimErr *Error
	require.ErrorAs(t, err, &scimErr)
	assert.Equal(t, ScimTypeMutability, scimErr.ScimType)
}

func TestUnknownAttributeRejected(t *testing.T) {
	err := patch(op(PatchOpAdd, "nonsense", `"x"`)).Apply(map[string]any{}, userSchemas())
	var scimErr *Error
	require.ErrorAs(t, err, &scimErr)
	assert.Equal(t, ScimTypeInvalidPath, scimErr.ScimType)
}

func TestUnknownSchemaURIRejected(t *testing.T) {
	err := patch(op(PatchOpReplace, string(core.SchemaUser)+"Bogus:userName", `"x"`)).Apply(map[string]any{}, userSchemas())
	var scimErr *Error
	require.ErrorAs(t, err, &scimErr)
	assert.Equal(t, ScimTypeInvalidPath, scimErr.ScimType)
}

func TestSchemaURISelectsSchema(t *testing.T) {
	doc := map[string]any{"userName": "old"}
	err := patch(op(PatchOpReplace, string(core.SchemaUser)+":userName", `"new"`)).Apply(doc, userSchemas())
	require.NoError(t, err)
	assert.Equal(t, "new", doc["userName"])
}

func TestApplyIsAtomicOnFailure(t *testing.T) {
	doc := map[string]any{"userName": "keep", "employeeNumber": "E1"}
	err := patch(
		op(PatchOpReplace, "userName", `"changed"`),
		op(PatchOpReplace, "employeeNumber", `"E2"`),
	).Apply(doc, userSchemas())
	var scimErr *Error
	require.ErrorAs(t, err, &scimErr)
	assert.Equal(t, ScimTypeMutability, scimErr.ScimType)
	assert.Equal(t, "keep", doc["userName"])
	assert.Equal(t, "E1", doc["employeeNumber"])
}

func TestInvalidOpRejected(t *testing.T) {
	err := patch(op(PatchOp("delete"), "userName", `"x"`)).Apply(map[string]any{}, nil)
	var scimErr *Error
	require.ErrorAs(t, err, &scimErr)
	assert.Equal(t, ScimTypeInvalidSyntax, scimErr.ScimType)
}

func TestApplyValuePathAndFilter(t *testing.T) {
	doc := map[string]any{"emails": []any{
		map[string]any{"type": "work", "primary": true, "value": "w@x"},
		map[string]any{"type": "work", "primary": false, "value": "w2@x"},
	}}
	err := patch(op(PatchOpReplace, `emails[type eq "work" and primary eq true].value`, `"new@x"`)).Apply(doc, nil)
	require.NoError(t, err)
	emails := doc["emails"].([]any)
	assert.Equal(t, "new@x", emails[0].(map[string]any)["value"])
	assert.Equal(t, "w2@x", emails[1].(map[string]any)["value"])
}

func TestApplyValuePathOrFilter(t *testing.T) {
	doc := map[string]any{"emails": []any{
		map[string]any{"type": "work", "value": "w@x"},
		map[string]any{"type": "home", "value": "h@x"},
		map[string]any{"type": "other", "value": "o@x"},
	}}
	err := patch(op(PatchOpRemove, `emails[type eq "work" or type eq "home"]`, "")).Apply(doc, nil)
	require.NoError(t, err)
	emails := doc["emails"].([]any)
	require.Len(t, emails, 1)
	assert.Equal(t, "other", emails[0].(map[string]any)["type"])
}

func TestApplyValuePathNotFilter(t *testing.T) {
	doc := map[string]any{"emails": []any{
		map[string]any{"type": "work", "value": "w@x"},
		map[string]any{"type": "home", "value": "h@x"},
	}}
	err := patch(op(PatchOpRemove, `emails[not (type eq "work")]`, "")).Apply(doc, nil)
	require.NoError(t, err)
	emails := doc["emails"].([]any)
	require.Len(t, emails, 1)
	assert.Equal(t, "work", emails[0].(map[string]any)["type"])
}

func TestApplyValuePathNumericFilter(t *testing.T) {
	doc := map[string]any{"scores": []any{
		map[string]any{"kind": "a", "n": float64(10)},
		map[string]any{"kind": "b", "n": float64(2)},
	}}
	err := patch(op(PatchOpRemove, `scores[n gt 5]`, "")).Apply(doc, nil)
	require.NoError(t, err)
	scores := doc["scores"].([]any)
	require.Len(t, scores, 1)
	assert.Equal(t, "b", scores[0].(map[string]any)["kind"])
}

func TestApplyValuePathPresenceFilter(t *testing.T) {
	doc := map[string]any{"emails": []any{
		map[string]any{"type": "work", "value": "w@x"},
		map[string]any{"type": "home"},
	}}
	err := patch(op(PatchOpRemove, `emails[value pr]`, "")).Apply(doc, nil)
	require.NoError(t, err)
	emails := doc["emails"].([]any)
	require.Len(t, emails, 1)
	assert.Equal(t, "home", emails[0].(map[string]any)["type"])
}

func TestApplyTypedUserPreservesUntouchedFields(t *testing.T) {
	user := &core.User{UserName: "old", DisplayName: "keep"}
	err := patch(op(PatchOpReplace, "userName", `"new"`)).Apply(user, nil)
	require.NoError(t, err)
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
	err := patch(op(PatchOpReplace, `emails[type eq "work"].value`, `"new@x"`)).Apply(user, nil)
	require.NoError(t, err)
	require.Len(t, user.Emails, 1)
	assert.Equal(t, "new@x", user.Emails[0].Value)
	require.NotNil(t, user.Active)
	assert.True(t, *user.Active)
}

func TestApplyTypedUserRemoveClearsField(t *testing.T) {
	user := &core.User{UserName: "bjensen", DisplayName: "Babs"}
	err := patch(op(PatchOpRemove, "displayName", "")).Apply(user, nil)
	require.NoError(t, err)
	assert.Empty(t, user.DisplayName)
	assert.Equal(t, "bjensen", user.UserName)
}
