package patch_test

import (
	"encoding/json"
	"fmt"
	"maps"
	"strings"
	"testing"
	"time"

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

func TestApplyPrimaryDemotesTheOtherValues(t *testing.T) {
	primaries := func(item core.Object) []any {
		emails := item["emails"].([]any)
		values := make([]any, len(emails))
		for i, email := range emails {
			values[i] = email.(map[string]any)["primary"]
		}
		return values
	}

	t.Run("when a value path sets primary", func(t *testing.T) {
		item := core.Object{"emails": []any{
			map[string]any{"type": "work", "primary": true},
			map[string]any{"type": "home"},
		}}

		require.NoError(t, apply(item, nil, operation(patch.OpReplace, `emails[type eq "home"].primary`, `true`)))

		assert.Equal(t, []any{false, true}, primaries(item))
	})

	t.Run("when an added value is primary", func(t *testing.T) {
		item := core.Object{"emails": []any{map[string]any{"type": "work", "primary": true}}}

		require.NoError(t, apply(item, nil, operation(patch.OpAdd, "emails", `[{"type":"home","primary":true}]`)))

		assert.Equal(t, []any{false, true}, primaries(item))
	})

	t.Run("but leaves the values of an attribute the patch does not make primary", func(t *testing.T) {
		item := core.Object{"active": false, "emails": []any{
			map[string]any{"type": "work", "primary": true},
			map[string]any{"type": "home", "primary": true},
		}}

		require.NoError(t, apply(item, nil, operation(patch.OpReplace, "active", `true`)))

		assert.Equal(t, []any{true, true}, primaries(item))
	})

	t.Run("when a value added without a path is primary", func(t *testing.T) {
		item := core.Object{"emails": []any{map[string]any{"type": "work", "primary": true}}}

		require.NoError(t, apply(item, nil, patch.Operation{Op: patch.OpAdd, Value: json.RawMessage(`{"emails":[{"type":"home","primary":true}]}`)}))

		assert.Equal(t, []any{false, true}, primaries(item))
	})

	t.Run("but keeps every value a value path makes primary", func(t *testing.T) {
		item := core.Object{"emails": []any{
			map[string]any{"type": "work"},
			map[string]any{"type": "work"},
			map[string]any{"type": "home", "primary": true},
		}}

		require.NoError(t, apply(item, nil, operation(patch.OpReplace, `emails[type eq "work"].primary`, `true`)))

		assert.Equal(t, []any{true, true, false}, primaries(item))
	})

	t.Run("but keeps two primaries the client sent in one value", func(t *testing.T) {
		item := core.Object{"emails": []any{}}

		require.NoError(t, apply(item, nil, operation(patch.OpAdd, "emails", `[{"primary":true},{"primary":true}]`)))

		assert.Equal(t, []any{true, true}, primaries(item))
	})
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
	item := map[string]any{"userName": "keep"}

	var err *scimerrors.Error
	require.ErrorAs(t, apply(item, userSchemas(),
		operation(patch.OpReplace, "userName", `"changed"`),
		operation(patch.OpReplace, "groups", `[{"value":"g1"}]`),
	), &err)

	assert.Equal(t, scimerrors.Mutability, err.ScimType)
	assert.Equal(t, map[string]any{"userName": "keep"}, item)
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

// RFC 7644 3.5.2.1 - adding to an absent multi-valued attribute must produce an array.
func TestApplyAddToAbsentMultiValuedWrapsInArray(t *testing.T) {
	item := map[string]any{}

	require.NoError(t, apply(item, userSchemas(), operation(patch.OpAdd, "emails", `{"value":"a@b.com"}`)))

	emails, ok := item["emails"].([]any)
	require.True(t, ok)
	require.Len(t, emails, 1)
}

// RFC 7644 Section 3.5.2: each operation against an attribute MUST be compatible with the attribute's mutability.
func TestApplyReadOnlySubAttributeIsRejected(t *testing.T) {
	schemas := []*core.Schema{
		(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("emails", core.TypeComplex).AsMultiValued().With(
				core.NewAttribute("type", core.TypeString).AsReadOnly(),
				core.NewAttribute("value", core.TypeString),
			),
			core.NewAttribute("name", core.TypeComplex).With(
				core.NewAttribute("givenName", core.TypeString),
				core.NewAttribute("formatted", core.TypeString).AsReadOnly(),
			),
		),
	}
	cases := []struct {
		name string
		op   patch.Operation
	}{
		{"a value-path merge", operation(patch.OpReplace, `emails[value eq "a@b.com"]`, `{"type":"home"}`)},
		{"a replaced array", operation(patch.OpReplace, "emails", `[{"value":"a@b.com","type":"home"}]`)},
		{"an added element", operation(patch.OpAdd, "emails", `{"value":"x@y.com","type":"home"}`)},
		{"an added array without a path", patch.Operation{Op: patch.OpAdd, Value: json.RawMessage(`{"emails":[{"value":"x@y.com","type":"home"}]}`)}},
		{"a merged complex attribute", operation(patch.OpReplace, "name", `{"formatted":"Ms. Barbara J Jensen"}`)},
		// RFC 7644 Section 3.5.2.2: a read-only attribute that is removed or becomes unassigned SHALL return "mutability".
		{"a removed complex attribute", operation(patch.OpRemove, "name", "")},
		{"a removed multi-valued attribute", operation(patch.OpRemove, "emails", "")},
		{"a removed element", operation(patch.OpRemove, `emails[value eq "a@b.com"]`, "")},
		{"a replaced array that drops a readOnly value", operation(patch.OpReplace, "emails", `[{"value":"a@b.com"}]`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := map[string]any{
				"emails": []any{map[string]any{"type": "work", "value": "a@b.com"}},
				"name":   map[string]any{"givenName": "Barbara", "formatted": "Barbara Jensen"},
			}

			requireMutability(t, apply(item, schemas, tc.op))
		})
	}

	t.Run("removes values whose readOnly sub-attributes are unassigned", func(t *testing.T) {
		item := map[string]any{
			"emails": []any{map[string]any{"value": "a@b.com"}, map[string]any{"value": "x@y.com", "type": ""}},
			"name":   map[string]any{"givenName": "Barbara"},
		}

		require.NoError(t, apply(item, schemas, operation(patch.OpRemove, `emails[value eq "a@b.com"]`, "")))
		require.NoError(t, apply(item, schemas, operation(patch.OpRemove, "emails", "")))
		require.NoError(t, apply(item, schemas, operation(patch.OpRemove, "name", "")))
		assert.Empty(t, item)
	})
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

	t.Run("does not add the extension object when removing from it", func(t *testing.T) {
		item := map[string]any{"userName": "bjensen"}

		require.NoError(t, apply(item, enterpriseSchemas(), operation(patch.OpRemove, uri+":department", "")))

		assert.Equal(t, map[string]any{"userName": "bjensen"}, item)
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

// RFC 7644 Section 3.5.2.2: if no other values remain after removal, the attribute SHALL be considered unassigned.
func TestApplyRemoveLastExtensionValueUnassignsTheExtension(t *testing.T) {
	uri := string(core.SchemaEnterpriseUser)
	item := map[string]any{"userName": "bjensen", uri: map[string]any{"department": "eng"}}

	require.NoError(t, apply(item, enterpriseSchemas(), operation(patch.OpRemove, uri+":department", "")))

	assert.Equal(t, map[string]any{"userName": "bjensen"}, item)
}

func TestApplyWithin(t *testing.T) {
	item := func() map[string]any {
		return map[string]any{"emails": []any{
			map[string]any{"value": "a"},
			map[string]any{"value": "b"},
			map[string]any{"value": "c"},
		}}
	}
	either := operation(patch.OpReplace, `emails[value eq "a" or value eq "b"].type`, `"work"`)
	present := operation(patch.OpReplace, `emails[value pr].type`, `"home"`)

	t.Run("applies a request whose value filter clauses times elements fit the budget", func(t *testing.T) {
		_, err := patch.ApplyWithin(item(), []patch.Operation{either}, userSchemas(), 6)
		require.NoError(t, err)
	})

	t.Run("refuses a request whose value filter clauses times elements exceed the budget with 413", func(t *testing.T) {
		_, err := patch.ApplyWithin(item(), []patch.Operation{either}, userSchemas(), 5)
		require.ErrorIs(t, err, scimerrors.ErrTooLarge(""))
	})

	t.Run("spends one budget across every operation", func(t *testing.T) {
		_, err := patch.ApplyWithin(item(), []patch.Operation{either, present}, userSchemas(), 9)
		require.NoError(t, err)

		_, err = patch.ApplyWithin(item(), []patch.Operation{either, present}, userSchemas(), 8)
		require.ErrorIs(t, err, scimerrors.ErrTooLarge(""))
	})

	t.Run("refuses before evaluating a filter that would match nothing", func(t *testing.T) {
		_, err := patch.ApplyWithin(item(), []patch.Operation{operation(patch.OpReplace, `emails[value eq "z"].type`, `"work"`)}, userSchemas(), 2)
		require.ErrorIs(t, err, scimerrors.ErrTooLarge(""))
	})

	t.Run("has no cap when the budget is zero", func(t *testing.T) {
		_, err := patch.ApplyWithin(item(), []patch.Operation{either, present}, userSchemas(), 0)
		require.NoError(t, err)
	})
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
					core.NewAttribute("formatted", core.TypeString).AsReadOnly(),
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

	t.Run("merges many distinct keys in linear time", func(t *testing.T) {
		const keyCount = 30000
		values := make(map[string]any, keyCount)
		for i := range keyCount {
			values[fmt.Sprintf("k%d", i)] = i
		}
		raw, err := json.Marshal(values)
		require.NoError(t, err)
		item := map[string]any{"name": map[string]any{}}

		start := time.Now()
		require.NoError(t, apply(item, schemas(), patch.Operation{Op: patch.OpAdd, Path: "name", Value: raw}))
		require.Less(t, time.Since(start), 3*time.Second)

		assert.Len(t, item["name"], keyCount)
	})
}

func apply(resource core.Object, schemas []*core.Schema, ops ...patch.Operation) error {
	patched, err := patch.Apply(resource, ops, schemas)
	if err != nil {
		return err
	}
	clear(resource)
	maps.Copy(resource, patched)
	return nil
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
		core.NewAttribute("employeeNumber", core.TypeString).AsReadOnly(),
		core.NewAttribute("manager", core.TypeComplex).With(
			core.NewAttribute("value", core.TypeString),
		),
	))
}

func extension(item map[string]any) map[string]any {
	nested, _ := item[string(core.SchemaEnterpriseUser)].(map[string]any)
	return nested
}

func requireMutability(t *testing.T, err error) {
	t.Helper()

	var scimErr *scimerrors.Error
	require.ErrorAs(t, err, &scimErr)
	assert.Equal(t, scimerrors.Mutability, scimErr.ScimType)
}
