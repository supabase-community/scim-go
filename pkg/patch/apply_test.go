package patch_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

func TestApplyTopLevel(t *testing.T) {
	item := map[string]any{"userName": "old"}

	require.NoError(t, apply(item, nil,
		operation(patch.OpReplace, "userName", `"new"`),
		operation(patch.OpAdd, "displayName", `"Babs"`),
	))

	assert.Equal(t, "new", item["userName"])
	assert.Equal(t, "Babs", item["displayName"])
}

func TestApplyTopLevelUser(t *testing.T) {
	item := &core.User{UserName: "old"}

	require.NoError(t, apply(item, nil,
		operation(patch.OpReplace, "userName", `"new"`),
		operation(patch.OpAdd, "displayName", `"Babs"`),
	))

	assert.Equal(t, "new", item.UserName)
	assert.Equal(t, "Babs", item.DisplayName)
}

func TestApplyTopLevelGroup(t *testing.T) {
	item := &core.Group{DisplayName: "Tour Guides"}

	require.NoError(t, apply(item, nil, operation(patch.OpReplace, "displayName", `"Example"`)))
	assert.Equal(t, "Example", item.DisplayName)
}

func TestApplyNoPathMerge(t *testing.T) {
	item := map[string]any{}

	require.NoError(t, apply(item, nil, patch.Operation{
		Op:    patch.OpAdd,
		Value: json.RawMessage(`{"nickName":"Babs","title":"Eng"}`),
	}))

	assert.Equal(t, "Babs", item["nickName"])
	assert.Equal(t, "Eng", item["title"])
}

func TestApplyAddAppendsAndIsCaseInsensitive(t *testing.T) {
	item := map[string]any{"emails": []any{map[string]any{"value": "a@b.com"}}}

	require.NoError(t, apply(item, nil, operation(patch.Op("Add"), "emails", `[{"value":"c@d.com"}]`)))

	assert.Len(t, item["emails"], 2)
}

func TestApplyAddSingleValueAppendsToArray(t *testing.T) {
	item := map[string]any{"emails": []any{map[string]any{"value": "a@b.com"}}}

	require.NoError(t, apply(item, nil, operation(patch.OpAdd, "emails", `{"value":"c@d.com"}`)))

	emails := item["emails"].([]any)
	require.Len(t, emails, 2)
	assert.Equal(t, "a@b.com", emails[0].(map[string]any)["value"])
	assert.Equal(t, "c@d.com", emails[1].(map[string]any)["value"])
}

func TestApplyMissingValueRejected(t *testing.T) {
	var err *scimerrors.Error
	require.ErrorAs(t, apply(map[string]any{}, nil, patch.Operation{Op: patch.OpReplace, Path: "userName"}), &err)

	assert.Equal(t, scimerrors.InvalidValue, err.ScimType)
}

func TestApplySubAttribute(t *testing.T) {
	item := map[string]any{"name": map[string]any{"familyName": "Old"}}

	require.NoError(t, apply(item, nil, operation(patch.OpReplace, "name.familyName", `"Jensen"`)))

	assert.Equal(t, "Jensen", item["name"].(map[string]any)["familyName"])
}

func TestApplyRemoveAttribute(t *testing.T) {
	item := map[string]any{"nickName": "Babs"}

	require.NoError(t, apply(item, nil, operation(patch.OpRemove, "nickName", "")))

	_, ok := item["nickName"]
	assert.False(t, ok)
}

func TestRemoveWithoutPathIsNoTarget(t *testing.T) {
	var err *scimerrors.Error
	require.ErrorAs(t, apply(map[string]any{}, nil, patch.Operation{Op: patch.OpRemove}), &err)

	assert.Equal(t, scimerrors.NoTarget, err.ScimType)
}

func TestApplyValuePathReplaceSub(t *testing.T) {
	item := map[string]any{"emails": []any{
		map[string]any{"type": "work", "value": "old@x"},
		map[string]any{"type": "home", "value": "h@x"},
	}}

	require.NoError(t, apply(item, nil, operation(patch.OpReplace, `emails[type eq "work"].value`, `"new@x"`)))

	emails := item["emails"].([]any)
	assert.Equal(t, "new@x", emails[0].(map[string]any)["value"])
	assert.Equal(t, "h@x", emails[1].(map[string]any)["value"])
}

func TestApplyValuePathRemoveElement(t *testing.T) {
	item := map[string]any{"emails": []any{
		map[string]any{"type": "work", "value": "w@x"},
		map[string]any{"type": "home", "value": "h@x"},
	}}

	require.NoError(t, apply(item, nil, operation(patch.OpRemove, `emails[type eq "home"]`, "")))

	assert.Len(t, item["emails"].([]any), 1)
}

func TestApplyValuePathRemoveSubAttribute(t *testing.T) {
	item := map[string]any{"emails": []any{
		map[string]any{"type": "work", "value": "w@x"},
		map[string]any{"type": "home", "value": "h@x"},
	}}

	require.NoError(t, apply(item, nil, operation(patch.OpRemove, `emails[type eq "work"].value`, "")))

	emails := item["emails"].([]any)
	assert.Equal(t, map[string]any{"type": "work"}, emails[0])
	assert.Equal(t, map[string]any{"type": "home", "value": "h@x"}, emails[1])
}

func TestApplyValuePathNoMatchIsNoTarget(t *testing.T) {
	item := map[string]any{"emails": []any{map[string]any{"type": "work"}}}

	var err *scimerrors.Error
	require.ErrorAs(t, apply(item, nil, operation(patch.OpReplace, `emails[type eq "home"].value`, `"x"`)), &err)

	assert.Equal(t, scimerrors.NoTarget, err.ScimType)
}

func TestReadOnlyIsSkipped(t *testing.T) {
	item := map[string]any{"groups": []any{}}

	require.NoError(t, apply(item, userSchemas(),
		operation(patch.OpReplace, "groups", `[{"value":"g1"}]`),
		operation(patch.OpReplace, "userName", `"bjensen"`),
	))

	assert.Empty(t, item["groups"])
	assert.Equal(t, "bjensen", item["userName"])
}

func TestIdIsProtected(t *testing.T) {
	item := map[string]any{"id": "keep"}

	require.NoError(t, apply(item, userSchemas(), operation(patch.OpReplace, "id", `"hacked"`)))
	assert.Equal(t, "keep", item["id"])
}

func TestImmutableAddWhenAbsentAllowed(t *testing.T) {
	item := map[string]any{}

	require.NoError(t, apply(item, userSchemas(), operation(patch.OpAdd, "employeeNumber", `"E1"`)))
	assert.Equal(t, "E1", item["employeeNumber"])
}

func TestImmutableReplaceWhenAbsentAllowed(t *testing.T) {
	item := map[string]any{}

	require.NoError(t, apply(item, userSchemas(), operation(patch.OpReplace, "employeeNumber", `"E1"`)))

	assert.Equal(t, "E1", item["employeeNumber"])
}

func TestApplyValuePathRelationalIsCaseInsensitive(t *testing.T) {
	item := map[string]any{"emails": []any{
		map[string]any{"type": "Zebra", "value": "z@x"},
		map[string]any{"type": "ant", "value": "a@x"},
	}}

	require.NoError(t, apply(item, nil, operation(patch.OpRemove, `emails[type gt "M"]`, "")))

	emails := item["emails"].([]any)
	require.Len(t, emails, 1)
	assert.Equal(t, "ant", emails[0].(map[string]any)["type"])
}

func TestImmutableChangeRejected(t *testing.T) {
	item := map[string]any{"employeeNumber": "E1"}

	var err *scimerrors.Error
	require.ErrorAs(t, apply(item, userSchemas(), operation(patch.OpReplace, "employeeNumber", `"E2"`)), &err)

	assert.Equal(t, scimerrors.Mutability, err.ScimType)
}

func TestUnknownAttributeRejected(t *testing.T) {
	var err *scimerrors.Error
	require.ErrorAs(t, apply(map[string]any{}, userSchemas(), operation(patch.OpAdd, "nonsense", `"x"`)), &err)

	assert.Equal(t, scimerrors.InvalidPath, err.ScimType)
}

func TestUnknownSchemaURIRejected(t *testing.T) {
	var err *scimerrors.Error
	require.ErrorAs(t, apply(map[string]any{}, userSchemas(), operation(patch.OpReplace, string(core.SchemaUser)+"Bogus:userName", `"x"`)), &err)

	assert.Equal(t, scimerrors.InvalidPath, err.ScimType)
}

func TestSchemaURISelectsSchema(t *testing.T) {
	item := map[string]any{"userName": "old"}

	require.NoError(t, apply(item, userSchemas(), operation(patch.OpReplace, string(core.SchemaUser)+":userName", `"new"`)))

	assert.Equal(t, "new", item["userName"])
}

func TestApplyIsAtomicOnFailure(t *testing.T) {
	item := map[string]any{"userName": "keep", "employeeNumber": "E1"}

	var err *scimerrors.Error
	require.ErrorAs(t, apply(item, userSchemas(),
		operation(patch.OpReplace, "userName", `"changed"`),
		operation(patch.OpReplace, "employeeNumber", `"E2"`),
	), &err)

	assert.Equal(t, scimerrors.Mutability, err.ScimType)
	assert.Equal(t, "keep", item["userName"])
	assert.Equal(t, "E1", item["employeeNumber"])
}

func TestInvalidOpRejected(t *testing.T) {
	var err *scimerrors.Error
	require.ErrorAs(t, apply(map[string]any{}, nil, operation(patch.Op("delete"), "userName", `"x"`)), &err)
	assert.Equal(t, scimerrors.InvalidSyntax, err.ScimType)
}

func TestApplyValuePathAndFilter(t *testing.T) {
	item := map[string]any{"emails": []any{
		map[string]any{"type": "work", "primary": true, "value": "w@x"},
		map[string]any{"type": "work", "primary": false, "value": "w2@x"},
	}}

	require.NoError(t, apply(item, nil, operation(patch.OpReplace, `emails[type eq "work" and primary eq true].value`, `"new@x"`)))

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

	require.NoError(t, apply(item, nil, operation(patch.OpRemove, `emails[type eq "work" or type eq "home"]`, "")))

	emails := item["emails"].([]any)
	require.Len(t, emails, 1)
	assert.Equal(t, "other", emails[0].(map[string]any)["type"])
}

func TestApplyValuePathNotFilter(t *testing.T) {
	item := map[string]any{"emails": []any{
		map[string]any{"type": "work", "value": "w@x"},
		map[string]any{"type": "home", "value": "h@x"},
	}}

	require.NoError(t, apply(item, nil, operation(patch.OpRemove, `emails[not (type eq "work")]`, "")))

	emails := item["emails"].([]any)
	require.Len(t, emails, 1)
	assert.Equal(t, "work", emails[0].(map[string]any)["type"])
}

func TestApplyValuePathNumericFilter(t *testing.T) {
	item := map[string]any{"scores": []any{
		map[string]any{"kind": "a", "n": float64(10)},
		map[string]any{"kind": "b", "n": float64(2)},
	}}

	require.NoError(t, apply(item, nil, operation(patch.OpRemove, `scores[n gt 5]`, "")))

	scores := item["scores"].([]any)
	require.Len(t, scores, 1)
	assert.Equal(t, "b", scores[0].(map[string]any)["kind"])
}

func TestApplyValuePathPresenceFilter(t *testing.T) {
	item := map[string]any{"emails": []any{
		map[string]any{"type": "work", "value": "w@x"},
		map[string]any{"type": "home"},
	}}

	require.NoError(t, apply(item, nil, operation(patch.OpRemove, `emails[value pr]`, "")))

	emails := item["emails"].([]any)
	require.Len(t, emails, 1)
	assert.Equal(t, "home", emails[0].(map[string]any)["type"])
}

func TestApplyTypedUserPreservesUntouchedFields(t *testing.T) {
	user := &core.User{UserName: "old", DisplayName: "keep"}

	require.NoError(t, apply(user, nil, operation(patch.OpReplace, "userName", `"new"`)))
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

	require.NoError(t, apply(user, nil, operation(patch.OpReplace, `emails[type eq "work"].value`, `"new@x"`)))
	require.Len(t, user.Emails, 1)
	assert.Equal(t, "new@x", user.Emails[0].Value)
	require.NotNil(t, user.Active)
	assert.True(t, *user.Active)
}

func TestApplyTypedUserRemoveClearsField(t *testing.T) {
	user := &core.User{UserName: "bjensen", DisplayName: "Babs"}

	require.NoError(t, apply(user, nil, operation(patch.OpRemove, "displayName", "")))
	assert.Empty(t, user.DisplayName)
	assert.Equal(t, "bjensen", user.UserName)
}

// RFC 7644 3.5.2.1 - adding to an absent multi-valued attribute must produce an array.
func TestApplyAddToAbsentMultiValuedWrapsInArray(t *testing.T) {
	item := map[string]any{}

	require.NoError(t, apply(item, userSchemas(), operation(patch.OpAdd, "emails", `{"value":"a@b.com"}`)))

	emails, ok := item["emails"].([]any)
	require.True(t, ok)
	require.Len(t, emails, 1)
}

// RFC 7643 7 - a value-path merge must honor each sub-attribute's mutability.
func TestApplyValuePathReplaceRespectsSubAttributeMutability(t *testing.T) {
	schemas := []*core.Schema{
		(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("emails", core.TypeComplex, "").AsMultiValued().With(
				core.NewAttribute("type", core.TypeString, "").AsImmutable(),
				core.NewAttribute("value", core.TypeString, ""),
			),
		),
	}
	item := map[string]any{"emails": []any{map[string]any{"type": "work", "value": "a@b.com"}}}

	var err *scimerrors.Error
	require.ErrorAs(t, apply(item, schemas, operation(patch.OpReplace, `emails[value eq "a@b.com"]`, `{"type":"home"}`)), &err)
	assert.Equal(t, scimerrors.Mutability, err.ScimType)
}

// RFC 7643 7 - a value-path merge into a readOnly complex attribute must be skipped.
func TestApplyValuePathMergeRespectsParentMutability(t *testing.T) {
	item := map[string]any{"groups": []any{map[string]any{"value": "g1"}}}

	require.NoError(t, apply(item, userSchemas(), operation(patch.OpReplace, `groups[value eq "g1"]`, `{"value":"g2"}`)))

	groups := item["groups"].([]any)
	assert.Equal(t, "g1", groups[0].(map[string]any)["value"])
}

// RFC 7643 7 - a value-path write to a sub-attribute of a readOnly attribute must be skipped.
func TestApplyValuePathWriteRespectsParentMutability(t *testing.T) {
	item := map[string]any{"groups": []any{map[string]any{"value": "g1"}}}

	require.NoError(t, apply(item, userSchemas(), operation(patch.OpReplace, `groups[value eq "g1"].value`, `"g2"`)))

	groups := item["groups"].([]any)
	assert.Equal(t, "g1", groups[0].(map[string]any)["value"])
}

// RFC 7643 7 - a value-path remove of a sub-attribute of a readOnly attribute must be skipped.
func TestApplyValuePathRemoveRespectsParentMutability(t *testing.T) {
	item := map[string]any{"groups": []any{map[string]any{"value": "g1"}}}

	require.NoError(t, apply(item, userSchemas(), operation(patch.OpRemove, `groups[value eq "g1"].value`, "")))

	groups := item["groups"].([]any)
	assert.Equal(t, "g1", groups[0].(map[string]any)["value"])
}

// RFC 7644 3.4.2.2 - a value filter on a caseExact attribute must compare case-sensitively.
func TestApplyValuePathFilterHonorsCaseExact(t *testing.T) {
	schemas := []*core.Schema{
		(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("emails", core.TypeComplex, "").AsMultiValued().With(
				core.NewAttribute("value", core.TypeString, "").AsCaseExact(),
			),
		),
	}
	item := map[string]any{"emails": []any{map[string]any{"value": "abc@x"}}}

	var err *scimerrors.Error
	require.ErrorAs(t, apply(item, schemas, operation(patch.OpRemove, `emails[value eq "ABC@x"]`, "")), &err)
	assert.Equal(t, scimerrors.NoTarget, err.ScimType)
}

func apply(resource any, schemas []*core.Schema, ops ...patch.Operation) error {
	return patch.Apply(resource, ops, schemas)
}

func operation(kind patch.Op, path, value string) patch.Operation {
	if value != "" {
		return patch.Operation{
			Op:    kind,
			Path:  path,
			Value: json.RawMessage(value),
		}
	}
	return patch.Operation{
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
