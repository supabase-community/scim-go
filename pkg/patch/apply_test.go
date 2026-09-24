package patch_test

import (
	"encoding/json"
	"strings"
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

// RFC 7644 Section 3.5.2: a client MUST NOT modify a readOnly attribute; the operation SHALL fail.
func TestReadOnlyIsRejected(t *testing.T) {
	item := map[string]any{"groups": []any{}}

	var err *scimerrors.Error
	require.ErrorAs(t, apply(item, userSchemas(),
		operation(patch.OpReplace, "userName", `"bjensen"`),
		operation(patch.OpReplace, "groups", `[{"value":"g1"}]`),
	), &err)

	assert.Equal(t, scimerrors.Mutability, err.ScimType)
	assert.Empty(t, item["groups"])
	assert.NotContains(t, item, "userName")
}

func TestIdIsProtected(t *testing.T) {
	item := map[string]any{"id": "keep"}

	var err *scimerrors.Error
	require.ErrorAs(t, apply(item, userSchemas(), operation(patch.OpReplace, "id", `"hacked"`)), &err)
	assert.Equal(t, scimerrors.Mutability, err.ScimType)
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
			core.NewAttribute("emails", core.TypeComplex).AsMultiValued().With(
				core.NewAttribute("type", core.TypeString).AsImmutable(),
				core.NewAttribute("value", core.TypeString),
			),
		),
	}
	item := map[string]any{"emails": []any{map[string]any{"type": "work", "value": "a@b.com"}}}

	var err *scimerrors.Error
	require.ErrorAs(t, apply(item, schemas, operation(patch.OpReplace, `emails[value eq "a@b.com"]`, `{"type":"home"}`)), &err)
	assert.Equal(t, scimerrors.Mutability, err.ScimType)
}

// RFC 7644 Section 3.5.2: a value-path merge into a readOnly complex attribute SHALL fail.
func TestApplyValuePathMergeRespectsParentMutability(t *testing.T) {
	item := map[string]any{"groups": []any{map[string]any{"value": "g1"}}}

	requireMutability(t, apply(item, userSchemas(), operation(patch.OpReplace, `groups[value eq "g1"]`, `{"value":"g2"}`)))

	groups := item["groups"].([]any)
	assert.Equal(t, "g1", groups[0].(map[string]any)["value"])
}

// RFC 7644 Section 3.5.2: a value-path write to a sub-attribute of a readOnly attribute SHALL fail.
func TestApplyValuePathWriteRespectsParentMutability(t *testing.T) {
	item := map[string]any{"groups": []any{map[string]any{"value": "g1"}}}

	requireMutability(t, apply(item, userSchemas(), operation(patch.OpReplace, `groups[value eq "g1"].value`, `"g2"`)))

	groups := item["groups"].([]any)
	assert.Equal(t, "g1", groups[0].(map[string]any)["value"])
}

// RFC 7644 Section 3.5.2: a value-path remove of a sub-attribute of a readOnly attribute SHALL fail.
func TestApplyValuePathRemoveRespectsParentMutability(t *testing.T) {
	item := map[string]any{"groups": []any{map[string]any{"value": "g1"}}}

	requireMutability(t, apply(item, userSchemas(), operation(patch.OpRemove, `groups[value eq "g1"].value`, "")))

	groups := item["groups"].([]any)
	assert.Equal(t, "g1", groups[0].(map[string]any)["value"])
}

// RFC 7644 3.4.2.2 - a value filter on a caseExact attribute must compare case-sensitively.
func TestApplyValuePathFilterHonorsCaseExact(t *testing.T) {
	schemas := []*core.Schema{
		(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("emails", core.TypeComplex).AsMultiValued().With(
				core.NewAttribute("value", core.TypeString).AsCaseExact(),
			),
		),
	}
	item := map[string]any{"emails": []any{map[string]any{"value": "abc@x"}}}

	var err *scimerrors.Error
	require.ErrorAs(t, apply(item, schemas, operation(patch.OpRemove, `emails[value eq "ABC@x"]`, "")), &err)
	assert.Equal(t, scimerrors.NoTarget, err.ScimType)
}

func TestApplyValuePathNumericFilterNativeInt(t *testing.T) {
	item := map[string]any{"scores": []any{
		map[string]any{"kind": "a", "n": 10},
		map[string]any{"kind": "b", "n": 2},
	}}

	require.NoError(t, apply(item, nil, operation(patch.OpRemove, `scores[n gt 5]`, "")))

	scores := item["scores"].([]any)
	require.Len(t, scores, 1)
	assert.Equal(t, "b", scores[0].(map[string]any)["kind"])
}

// RFC 7644 3.4.2.2 - pr does not match an empty value.
func TestApplyValuePathPresenceIgnoresEmptyString(t *testing.T) {
	item := map[string]any{"emails": []any{
		map[string]any{"type": "work", "value": "w@x"},
		map[string]any{"type": "home", "value": ""},
	}}

	require.NoError(t, apply(item, nil, operation(patch.OpRemove, `emails[value pr]`, "")))

	emails := item["emails"].([]any)
	require.Len(t, emails, 1)
	assert.Equal(t, "home", emails[0].(map[string]any)["type"])
}

func TestApplyRemoveSubAttributeAcrossMultiValued(t *testing.T) {
	item := map[string]any{"emails": []any{
		map[string]any{"type": "work", "value": "w@x"},
		map[string]any{"type": "home", "value": "h@x"},
	}}

	require.NoError(t, apply(item, nil, operation(patch.OpRemove, "emails.type", "")))

	emails := item["emails"].([]any)
	require.Len(t, emails, 2)
	assert.Equal(t, map[string]any{"value": "w@x"}, emails[0])
	assert.Equal(t, map[string]any{"value": "h@x"}, emails[1])
}

func TestApplyRemoveSubAttributeMultiValuedNoTarget(t *testing.T) {
	item := map[string]any{"emails": []any{}}

	var err *scimerrors.Error
	require.ErrorAs(t, apply(item, nil, operation(patch.OpRemove, "emails.type", "")), &err)
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
			core.NewAttribute("userName", core.TypeString),
			core.NewAttribute("displayName", core.TypeString),
			core.NewAttribute("employeeNumber", core.TypeString).AsImmutable(),
			core.NewAttribute("groups", core.TypeComplex).AsMultiValued().AsReadOnly().With(
				core.NewAttribute("value", core.TypeString),
			),
			core.NewAttribute("name", core.TypeComplex).With(
				core.NewAttribute("familyName", core.TypeString),
			),
			core.NewAttribute("emails", core.TypeComplex).AsMultiValued().With(
				core.NewAttribute("type", core.TypeString),
				core.NewAttribute("value", core.TypeString),
			),
		),
	}
}

func enterpriseSchemas() []*core.Schema {
	return append(userSchemas(), (&core.Schema{ID: core.SchemaEnterpriseUser}).With(
		core.NewAttribute("department", core.TypeString),
		core.NewAttribute("employeeNumber", core.TypeString).AsImmutable(),
		core.NewAttribute("manager", core.TypeComplex).With(
			core.NewAttribute("value", core.TypeString),
		),
	))
}

func extension(item map[string]any) map[string]any {
	nested, _ := item[string(core.SchemaEnterpriseUser)].(map[string]any)
	return nested
}

// RFC 7644 Section 3.10: a URN-qualified path reaches into the schema extension object.
func TestApplyExtensionPath(t *testing.T) {
	uri := string(core.SchemaEnterpriseUser)

	t.Run("replaces an extension attribute", func(t *testing.T) {
		item := map[string]any{uri: map[string]any{"department": "eng"}}

		require.NoError(t, apply(item, enterpriseSchemas(), operation(patch.OpReplace, uri+":department", `"sales"`)))

		assert.Equal(t, map[string]any{"department": "sales"}, extension(item))
		assert.NotContains(t, item, "department")
	})

	t.Run("adds the extension object when it is absent", func(t *testing.T) {
		item := map[string]any{}

		require.NoError(t, apply(item, enterpriseSchemas(), operation(patch.OpAdd, uri+":manager.value", `"42"`)))

		assert.Equal(t, map[string]any{"manager": map[string]any{"value": "42"}}, extension(item))
	})

	t.Run("matches the URN case-insensitively", func(t *testing.T) {
		item := map[string]any{uri: map[string]any{"department": "eng"}}

		require.NoError(t, apply(item, enterpriseSchemas(), operation(patch.OpReplace, strings.ToUpper(uri)+":department", `"sales"`)))

		assert.Equal(t, map[string]any{"department": "sales"}, extension(item))
		assert.Len(t, item, 1)
	})

	t.Run("removes an extension attribute", func(t *testing.T) {
		item := map[string]any{"department": "top", uri: map[string]any{"department": "eng", "employeeNumber": "E1"}}

		require.NoError(t, apply(item, enterpriseSchemas(), operation(patch.OpRemove, uri+":department", "")))

		assert.Equal(t, map[string]any{"employeeNumber": "E1"}, extension(item))
		assert.Equal(t, "top", item["department"])
	})

	t.Run("enforces the mutability of an extension attribute", func(t *testing.T) {
		item := map[string]any{uri: map[string]any{"employeeNumber": "E1"}}

		var err *scimerrors.Error
		require.ErrorAs(t, apply(item, enterpriseSchemas(), operation(patch.OpReplace, uri+":employeeNumber", `"E2"`)), &err)
		assert.Equal(t, scimerrors.Mutability, err.ScimType)
	})

	t.Run("does not resolve common attributes through an extension", func(t *testing.T) {
		var err *scimerrors.Error
		require.ErrorAs(t, apply(map[string]any{}, enterpriseSchemas(), operation(patch.OpReplace, uri+":externalId", `"x"`)), &err)
		assert.Equal(t, scimerrors.InvalidPath, err.ScimType)
	})

	t.Run("merges an extension object when the path is omitted", func(t *testing.T) {
		item := map[string]any{uri: map[string]any{"employeeNumber": "E1"}}

		require.NoError(t, apply(item, enterpriseSchemas(), patch.Operation{
			Op:    patch.OpReplace,
			Value: json.RawMessage(`{"userName":"bjensen","` + uri + `":{"department":"ops"}}`),
		}))

		assert.Equal(t, "bjensen", item["userName"])
		assert.Equal(t, map[string]any{"employeeNumber": "E1", "department": "ops"}, extension(item))
	})

	t.Run("writes a URN-qualified key when the path is omitted", func(t *testing.T) {
		item := map[string]any{}

		require.NoError(t, apply(item, enterpriseSchemas(), patch.Operation{
			Op:    patch.OpReplace,
			Value: json.RawMessage(`{"` + uri + `:department":"hr","` + string(core.SchemaUser) + `:userName":"bjensen"}`),
		}))

		assert.Equal(t, "bjensen", item["userName"])
		assert.Equal(t, map[string]any{"department": "hr"}, extension(item))
	})
}

func requireMutability(t *testing.T, err error) {
	t.Helper()

	var scimErr *scimerrors.Error
	require.ErrorAs(t, err, &scimErr)
	assert.Equal(t, scimerrors.Mutability, scimErr.ScimType)
}

func TestApplyRemoveReadOnlyRejected(t *testing.T) {
	item := map[string]any{"groups": []any{map[string]any{"value": "g1"}}}

	requireMutability(t, apply(item, userSchemas(), operation(patch.OpRemove, "groups", "")))
	assert.Len(t, item["groups"], 1)
}

// RFC 7644 Section 3.5.2.3: sub-attributes that are not specified in the "value" parameter are left unchanged.
func TestApplyComplexMerge(t *testing.T) {
	schemas := func() []*core.Schema {
		return []*core.Schema{
			(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
				core.NewAttribute("name", core.TypeComplex).With(
					core.NewAttribute("givenName", core.TypeString),
					core.NewAttribute("familyName", core.TypeString),
					core.NewAttribute("formatted", core.TypeString).AsImmutable(),
				),
			),
		}
	}
	existing := func() map[string]any {
		return map[string]any{"name": map[string]any{"givenName": "Barbara", "familyName": "Jensen"}}
	}

	for _, kind := range []patch.Op{patch.OpReplace, patch.OpAdd} {
		t.Run(string(kind)+" with a path merges sub-attributes", func(t *testing.T) {
			item := existing()

			require.NoError(t, apply(item, schemas(), operation(kind, "name", `{"givenName":"Babs"}`)))

			assert.Equal(t, map[string]any{"givenName": "Babs", "familyName": "Jensen"}, item["name"])
		})

		t.Run(string(kind)+" without a path merges sub-attributes", func(t *testing.T) {
			item := existing()

			require.NoError(t, apply(item, schemas(), patch.Operation{Op: kind, Value: json.RawMessage(`{"name":{"givenName":"Babs"}}`)}))

			assert.Equal(t, map[string]any{"givenName": "Babs", "familyName": "Jensen"}, item["name"])
		})
	}

	t.Run("creates the complex attribute when it is absent", func(t *testing.T) {
		item := map[string]any{}

		require.NoError(t, apply(item, schemas(), operation(patch.OpReplace, "name", `{"givenName":"Babs"}`)))

		assert.Equal(t, map[string]any{"givenName": "Babs"}, item["name"])
	})

	t.Run("enforces the mutability of each merged sub-attribute", func(t *testing.T) {
		item := map[string]any{"name": map[string]any{"formatted": "Ms. Barbara J Jensen"}}

		requireMutability(t, apply(item, schemas(), operation(patch.OpReplace, "name", `{"formatted":"Babs"}`)))
	})

	t.Run("replaces a multi-valued attribute as a whole", func(t *testing.T) {
		item := map[string]any{"emails": []any{map[string]any{"value": "a@b.com"}}}

		require.NoError(t, apply(item, userSchemas(), operation(patch.OpReplace, "emails", `[{"value":"c@d.com"}]`)))

		assert.Equal(t, []any{map[string]any{"value": "c@d.com"}}, item["emails"])
	})
}
