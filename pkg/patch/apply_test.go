package patch_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
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

// RFC 7644 Section 3.5.2.1: if the target location already contains the value specified, no changes SHOULD be made.
func TestApplyAddSkipsAValueTheTargetAlreadyContains(t *testing.T) {
	item := core.Object{"emails": []any{map[string]any{"type": "work", "value": "a@b.com"}}}

	require.NoError(t, apply(item, userSchemas(), operation(patch.OpAdd, "emails", `[{"type":"work","value":"a@b.com"},{"type":"home","value":"c@d.com"}]`)))

	assert.Len(t, item["emails"], 2)
}

// RFC 7644 Section 3.5.2.1: if the target location already contains the value specified, no changes SHOULD be made.
func TestApplyAddWithoutPathSkipsAValueTheTargetAlreadyContains(t *testing.T) {
	item := core.Object{"emails": []any{map[string]any{"type": "work", "value": "a@b.com"}}}

	require.NoError(t, apply(item, userSchemas(), patch.Operation{Op: patch.OpAdd, Value: json.RawMessage(`{"emails":[{"type":"work","value":"a@b.com"}]}`)}))

	assert.Len(t, item["emails"], 1)
}

// RFC 7644 Section 3.5.2.1: if the target location already contains the value specified, no changes SHOULD be made.
func TestApplyAddSkipsAnElementAlreadyPresentByValueAlone(t *testing.T) {
	item := core.Object{"members": []any{map[string]any{"value": "u-1", "type": "User", "$ref": "https://example.com/Users/u-1"}}}

	require.NoError(t, apply(item, groupSchemas(), operation(patch.OpAdd, "members", `[{"value":"u-1","display":"Bob"}]`)))

	assert.Len(t, item["members"], 1)
}

// RFC 7643 Section 2.2: caseExact defaults to false, so an add matches an existing element regardless of case.
func TestApplyAddSkipsAnElementAlreadyPresentByValueCaseInsensitively(t *testing.T) {
	item := core.Object{"members": []any{map[string]any{"value": "u-1", "type": "User"}}}

	require.NoError(t, apply(item, groupSchemas(), operation(patch.OpAdd, "members", `[{"value":"U-1"}]`)))

	assert.Len(t, item["members"], 1)
}

// RFC 7643 Section 2.4: the same "value" MAY repeat under a different "type", so adding one is not a no-op.
func TestApplyAddDoesNotSkipTheSameValueUnderADifferentType(t *testing.T) {
	item := core.Object{"emails": []any{map[string]any{"value": "a@b.com", "type": "work"}}}

	require.NoError(t, apply(item, userSchemas(), operation(patch.OpAdd, "emails", `[{"value":"a@b.com","type":"home"}]`)))

	assert.Len(t, item["emails"], 2)
}

// RFC 7644 Section 3.5.2.1: if the target location already contains the value specified, no changes SHOULD be made.
func TestApplyAddSkipsTheSameValueAndTypeCaseInsensitively(t *testing.T) {
	item := core.Object{"emails": []any{map[string]any{"value": "a@b.com", "type": "work"}}}

	require.NoError(t, apply(item, userSchemas(), operation(patch.OpAdd, "emails", `[{"value":"A@B.com","type":"Work"}]`)))

	assert.Len(t, item["emails"], 1)
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

	t.Run("but leaves primary untouched when the added value duplicates the existing one", func(t *testing.T) {
		item := core.Object{"emails": []any{map[string]any{"type": "work", "value": "a@b.com", "primary": true}}}

		require.NoError(t, apply(item, userSchemas(), operation(patch.OpAdd, "emails", `[{"type":"work","value":"a@b.com","primary":true}]`)))

		assert.Equal(t, []any{true}, primaries(item))
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

// RFC 7644 Section 3.5.2.3: a value filter that matches several elements replaces the sub-attribute of each.
func TestApplyValuePathGivesEachMatchedElementItsOwnValue(t *testing.T) {
	schemas := []*core.Schema{(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
		core.NewAttribute("emails", core.TypeComplex).AsMultiValued().With(
			core.NewAttribute("value", core.TypeString),
			core.NewAttribute("type", core.TypeString),
			core.NewAttribute("tags", core.TypeString).AsMultiValued(),
		),
	)}

	for _, op := range []patch.Operation{
		operation(patch.OpReplace, `emails[type eq "work"].tags`, `["x","y"]`),
		operation(patch.OpReplace, `emails[type eq "work"]`, `{"tags":["x","y"]}`),
	} {
		item := map[string]any{"emails": []any{
			map[string]any{"value": "a", "type": "work"},
			map[string]any{"value": "b", "type": "work"},
		}}

		require.NoError(t, apply(item, schemas, op), op.Path)

		emails := item["emails"].([]any)
		first, second := emails[0].(map[string]any)["tags"].([]any), emails[1].(map[string]any)["tags"].([]any)
		assert.Equal(t, []any{"x", "y"}, first, op.Path)
		assert.Equal(t, []any{"x", "y"}, second, op.Path)
		assert.NotSame(t, &first[0], &second[0], op.Path)
	}
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

// RFC 7643 Section 2.4: multi-valued attributes contain a list of elements using the JSON array format.
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

	require.NoError(t, apply(item, schemas, operation(patch.OpRemove, `emails[value eq "ABC@x"]`, "")))

	emails := item["emails"].([]any)
	require.Len(t, emails, 1)
	assert.Equal(t, "abc@x", emails[0].(map[string]any)["value"])
}

// RFC 7644 Section 3.5.2.2: removing a value a filter doesn't match makes no change and still succeeds.
func TestApplyValuePathRemoveNoMatchIsANoOp(t *testing.T) {
	item := map[string]any{"emails": []any{map[string]any{"type": "work", "value": "w@x"}}}

	require.NoError(t, apply(item, nil, operation(patch.OpRemove, `emails[type eq "home"]`, "")))

	assert.Len(t, item["emails"].([]any), 1)
}

// RFC 7643 Section 2.5: unassigned attributes and the null value SHALL be considered equivalent.
func TestApplyValuePathNullFilter(t *testing.T) {
	item := map[string]any{"emails": []any{
		map[string]any{"type": "work", "value": "w@x"},
		map[string]any{"type": "home"},
	}}

	require.NoError(t, apply(item, nil, operation(patch.OpRemove, `emails[value eq null]`, "")))

	emails := item["emails"].([]any)
	require.Len(t, emails, 1)
	assert.Equal(t, "work", emails[0].(map[string]any)["type"])
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

func TestApplyRemoveLastExtensionValueUnassignsTheExtension(t *testing.T) {
	uri := string(core.SchemaEnterpriseUser)
	item := map[string]any{"userName": "bjensen", uri: map[string]any{"department": "eng"}}

	require.NoError(t, apply(item, enterpriseSchemas(), operation(patch.OpRemove, uri+":department", "")))

	assert.Equal(t, map[string]any{"userName": "bjensen"}, item)
}

func TestApplyMaxFilterEvaluations(t *testing.T) {
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
		err := patch.Apply(item(), []patch.Operation{either}, userSchemas(), patch.MaxFilterEvaluations(6))
		require.NoError(t, err)
	})

	t.Run("refuses a request whose value filter clauses times elements exceed the budget with 413", func(t *testing.T) {
		err := patch.Apply(item(), []patch.Operation{either}, userSchemas(), patch.MaxFilterEvaluations(5))
		require.ErrorIs(t, err, scimerrors.ErrTooLarge(""))
	})

	t.Run("spends one budget across every operation", func(t *testing.T) {
		err := patch.Apply(item(), []patch.Operation{either, present}, userSchemas(), patch.MaxFilterEvaluations(9))
		require.NoError(t, err)

		err = patch.Apply(item(), []patch.Operation{either, present}, userSchemas(), patch.MaxFilterEvaluations(8))
		require.ErrorIs(t, err, scimerrors.ErrTooLarge(""))
	})

	t.Run("refuses before evaluating a filter that would match nothing", func(t *testing.T) {
		err := patch.Apply(item(), []patch.Operation{operation(patch.OpReplace, `emails[value eq "z"].type`, `"work"`)}, userSchemas(), patch.MaxFilterEvaluations(2))
		require.ErrorIs(t, err, scimerrors.ErrTooLarge(""))
	})

	t.Run("has no cap when the budget is zero", func(t *testing.T) {
		err := patch.Apply(item(), []patch.Operation{either, present}, userSchemas(), patch.MaxFilterEvaluations(0))
		require.NoError(t, err)
	})

	t.Run("refuses a value-filtered write once holders times value size exceed the budget", func(t *testing.T) {
		const holderCount = 2000
		elements := make([]map[string]any, holderCount)
		for i := range elements {
			elements[i] = map[string]any{"type": "a", "n": i}
		}
		raw, err := json.Marshal(elements)
		require.NoError(t, err)

		big := strings.Repeat("x", 6000)
		err = patch.Apply(map[string]any{}, []patch.Operation{
			operation(patch.OpAdd, "emails", string(raw)),
			operation(patch.OpReplace, `emails[type eq "a"].value`, `"`+big+`"`),
		}, userSchemas(), patch.MaxFilterEvaluations(10_000_000))

		require.ErrorIs(t, err, scimerrors.ErrTooLarge(""))
	})

	t.Run("allows an ordinary value-filtered write within the budget", func(t *testing.T) {
		resource := map[string]any{"emails": []any{
			map[string]any{"type": "a", "value": "old-1"},
			map[string]any{"type": "a", "value": "old-2"},
		}}

		err := patch.Apply(resource, []patch.Operation{
			operation(patch.OpReplace, `emails[type eq "a"].value`, `"new@example.com"`),
		}, userSchemas(), patch.MaxFilterEvaluations(10_000_000))
		require.NoError(t, err)

		emails := resource["emails"].([]any)
		require.Len(t, emails, 2)
		for _, e := range emails {
			assert.Equal(t, "new@example.com", e.(map[string]any)["value"])
		}
	})

	t.Run("charges a clause for each nested not, not only for the leaf it wraps", func(t *testing.T) {
		nested := strings.Repeat("not(", 50) + `value eq "a"` + strings.Repeat(")", 50)
		op := operation(patch.OpReplace, `emails[`+nested+`].type`, `"work"`)

		err := patch.Apply(item(), []patch.Operation{op}, userSchemas(), patch.MaxFilterEvaluations(4))
		require.ErrorIs(t, err, scimerrors.ErrTooLarge(""))

		err = patch.Apply(item(), []patch.Operation{op}, userSchemas(), patch.MaxFilterEvaluations(200))
		require.NoError(t, err)
	})
}

// RFC 7644 Section 3.5.2.1: if the target already contains the value, no changes SHOULD be made to the resource.
func TestApplyDedupAcrossOperationsInOneRequest(t *testing.T) {
	item := map[string]any{"emails": []any{map[string]any{"value": "a@x"}}}

	require.NoError(t, apply(item, userSchemas(),
		operation(patch.OpAdd, "emails", `[{"value":"b@x"}]`),
		operation(patch.OpAdd, "emails", `[{"value":"b@x"}]`),
	))

	assert.ElementsMatch(t, []any{map[string]any{"value": "a@x"}, map[string]any{"value": "b@x"}}, item["emails"])
}

// RFC 7644 Section 3.5.2.1: if the target already contains the value, no changes SHOULD be made to the resource.
func TestApplyDedupInvalidatedByAReplaceOfTheWholeAttribute(t *testing.T) {
	item := map[string]any{"emails": []any{map[string]any{"value": "a@x"}}}

	require.NoError(t, apply(item, userSchemas(),
		operation(patch.OpReplace, "emails", `[{"value":"b@x"}]`),
		operation(patch.OpAdd, "emails", `[{"value":"a@x"}]`),
	))

	assert.ElementsMatch(t, []any{map[string]any{"value": "b@x"}, map[string]any{"value": "a@x"}}, item["emails"])
}

// RFC 7644 Section 3.5.2.1: if the target already contains the value, no changes SHOULD be made to the resource.
func TestApplyDedupInvalidatedByARemoveOfTheWholeAttribute(t *testing.T) {
	item := map[string]any{"emails": []any{map[string]any{"value": "a@x"}}}

	require.NoError(t, apply(item, userSchemas(),
		operation(patch.OpRemove, "emails", ""),
		operation(patch.OpAdd, "emails", `[{"value":"a@x"}]`),
	))

	assert.Equal(t, []any{map[string]any{"value": "a@x"}}, item["emails"])
}

// RFC 7644 Section 3.5.2.1: if the target already contains the value, no changes SHOULD be made to the resource.
func TestApplyDedupInvalidatedByAValueFilteredRemove(t *testing.T) {
	item := map[string]any{"emails": []any{map[string]any{"value": "a@x"}, map[string]any{"value": "b@x"}}}

	require.NoError(t, apply(item, userSchemas(),
		operation(patch.OpRemove, `emails[value eq "a@x"]`, ""),
		operation(patch.OpAdd, "emails", `[{"value":"a@x"}]`),
	))

	assert.ElementsMatch(t, []any{map[string]any{"value": "b@x"}, map[string]any{"value": "a@x"}}, item["emails"])
}

// RFC 7644 Section 3.5.2.1: if the target location already contains the value specified, no changes SHOULD be made.
func TestApplyDedupAcrossOperationsWithoutAValueSubAttribute(t *testing.T) {
	schemas := []*core.Schema{
		(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("addresses", core.TypeComplex).AsMultiValued().With(
				core.NewAttribute("streetAddress", core.TypeString),
			),
		),
	}
	item := map[string]any{"addresses": []any{map[string]any{"streetAddress": "1 Main"}}}

	require.NoError(t, apply(item, schemas,
		operation(patch.OpAdd, "addresses", `[{"streetAddress":"2 Main"}]`),
		operation(patch.OpAdd, "addresses", `[{"streetAddress":"2 Main"}]`),
	))

	assert.ElementsMatch(t, []any{map[string]any{"streetAddress": "1 Main"}, map[string]any{"streetAddress": "2 Main"}}, item["addresses"])
}

func TestApplyDedupAfterAValueFilteredReplaceOfAMatchedElement(t *testing.T) {
	item := map[string]any{"emails": []any{map[string]any{"value": "a@x"}}}

	require.NoError(t, apply(item, userSchemas(),
		operation(patch.OpAdd, "emails", `[{"value":"b@x"}]`),
		operation(patch.OpReplace, `emails[value eq "a@x"].value`, `"c@x"`),
		operation(patch.OpAdd, "emails", `[{"value":"a@x"}]`),
	))

	assert.ElementsMatch(t, []any{map[string]any{"value": "c@x"}, map[string]any{"value": "b@x"}, map[string]any{"value": "a@x"}}, item["emails"])
}

func TestApplyDedupAfterDemoteOfAnExistingElement(t *testing.T) {
	schemas := []*core.Schema{
		(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("addresses", core.TypeComplex).AsMultiValued().With(
				core.NewAttribute("streetAddress", core.TypeString),
				core.NewAttribute("primary", core.TypeBoolean),
			),
		),
	}
	item := map[string]any{}

	require.NoError(t, apply(item, schemas,
		operation(patch.OpAdd, "addresses", `[{"streetAddress":"1","primary":true}]`),
		operation(patch.OpAdd, "addresses", `[{"streetAddress":"2","primary":true}]`),
		operation(patch.OpAdd, "addresses", `[{"streetAddress":"1","primary":false}]`),
	))

	assert.ElementsMatch(t, []any{
		map[string]any{"streetAddress": "1", "primary": false},
		map[string]any{"streetAddress": "2", "primary": true},
	}, item["addresses"])
}

func TestApplyMaxWriteBytes(t *testing.T) {
	item := func() map[string]any {
		return map[string]any{"emails": []any{map[string]any{"type": "a", "value": "old"}}}
	}
	write := func(value string) patch.Operation {
		return operation(patch.OpReplace, `emails[type eq "a"].value`, `"`+value+`"`)
	}

	t.Run("refuses a write once holders times value size exceed the configured budget", func(t *testing.T) {
		err := patch.Apply(item(), []patch.Operation{write(strings.Repeat("x", 100))}, userSchemas(), patch.MaxWriteBytes(50))
		require.ErrorIs(t, err, scimerrors.ErrTooLarge(""))
	})

	t.Run("allows the same write within a larger configured budget", func(t *testing.T) {
		err := patch.Apply(item(), []patch.Operation{write(strings.Repeat("x", 100))}, userSchemas(), patch.MaxWriteBytes(1000))
		require.NoError(t, err)
	})

	t.Run("has no cap when the budget is zero", func(t *testing.T) {
		err := patch.Apply(item(), []patch.Operation{write(strings.Repeat("x", 10<<20))}, userSchemas(), patch.MaxWriteBytes(0))
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

func TestApplyValueFilterRecomputesLiteralPerElement(t *testing.T) {
	certSchemas := []*core.Schema{
		(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("certs", core.TypeComplex).AsMultiValued().With(
				core.NewAttribute("value", core.TypeBinary),
			),
		),
	}
	const elementCount = 2000
	elements := make([]map[string]any, elementCount)
	for i := range elements {
		elements[i] = map[string]any{}
	}
	raw, err := json.Marshal(elements)
	require.NoError(t, err)

	run := func(literal string) time.Duration {
		item := core.Object{}
		require.NoError(t, apply(item, certSchemas, operation(patch.OpAdd, "certs", string(raw))))

		start := time.Now()
		err := apply(item, certSchemas, operation(patch.OpRemove, fmt.Sprintf(`certs[value eq %q]`, literal), ""))
		elapsed := time.Since(start)

		require.NoError(t, err)
		return elapsed
	}

	small := run(base64.StdEncoding.EncodeToString([]byte("x")))
	large := run(base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 6000))))

	assert.Less(t, large, 10*small, "a value filter's per-element cost must not scale with the constant literal's size; it is re-decoded once per element instead of once per operation")
}

func TestApplyAddWithoutAValueSubAttributeDoesNotScaleQuadratically(t *testing.T) {
	addressSchemas := []*core.Schema{core.NewSchema(core.SchemaUser).With(core.UserAttributes()...)}

	run := func(n int) time.Duration {
		item := core.Object{}
		first := make([]map[string]any, n)
		for i := range first {
			first[i] = map[string]any{"postalCode": fmt.Sprintf("a%d", i)}
		}
		raw, err := json.Marshal(first)
		require.NoError(t, err)
		require.NoError(t, apply(item, addressSchemas, operation(patch.OpAdd, "addresses", string(raw))))

		second := make([]map[string]any, n)
		for i := range second {
			second[i] = map[string]any{"postalCode": fmt.Sprintf("b%d", i)}
		}
		raw, err = json.Marshal(second)
		require.NoError(t, err)

		start := time.Now()
		err = apply(item, addressSchemas, operation(patch.OpAdd, "addresses", string(raw)))
		elapsed := time.Since(start)

		require.NoError(t, err)
		return elapsed
	}

	small := run(200)
	large := run(2000)

	assert.Less(t, large, 30*small, "adding N elements without a \"value\" sub-attribute must scale close to N, not N^2")
}

// RFC 7644 Section 3.5.2: a client MUST NOT modify an attribute that has mutability "immutable".
func TestApplyReplaceMembersKeepsOmittedImmutableSubAttributes(t *testing.T) {
	item := core.Object{"members": []any{map[string]any{"value": "u-1", "type": "User"}}}

	require.NoError(t, apply(item, groupSchemas(), operation(patch.OpReplace, "members", `[{"value":"u-1"}]`)))

	members := item["members"].([]any)
	require.Len(t, members, 1)
	assert.Equal(t, "User", members[0].(map[string]any)["type"])
}

// RFC 7643 Section 2.2: caseExact defaults to false, so "value" matches the stored element regardless of case.
func TestApplyReplaceMembersMatchesTheStoredValueCaseInsensitively(t *testing.T) {
	item := core.Object{"members": []any{map[string]any{"value": "u-1", "type": "User"}}}

	require.NoError(t, apply(item, groupSchemas(), operation(patch.OpReplace, "members", `[{"value":"U-1"}]`)))

	members := item["members"].([]any)
	require.Len(t, members, 1)
	assert.Equal(t, "User", members[0].(map[string]any)["type"])
}

// RFC 7643 Section 2.1: attribute names are case insensitive, so a stored sub-attribute keyed with different case is still found.
func TestApplyReplaceMembersKeepsAStoredSubAttributeKeyedWithDifferentCase(t *testing.T) {
	item := core.Object{"members": []any{map[string]any{"value": "u-1", "Type": "User"}}}

	require.NoError(t, apply(item, groupSchemas(), operation(patch.OpReplace, "members", `[{"value":"u-1"}]`)))

	members := item["members"].([]any)
	require.Len(t, members, 1)
	assert.Equal(t, "User", core.Object(members[0].(map[string]any)).Get("type"))
}

// RFC 7643 Section 2.1: an explicit sub-attribute keyed with different case is not overwritten by carry-forward.
func TestApplyReplaceMembersLeavesAnExplicitSubAttributeKeyedWithDifferentCaseAlone(t *testing.T) {
	item := core.Object{"members": []any{map[string]any{"value": "u-1", "type": "User"}}}

	require.NoError(t, apply(item, groupSchemas(), operation(patch.OpReplace, "members", `[{"value":"u-1","Type":"Group"}]`)))

	members := item["members"].([]any)
	require.Len(t, members, 1)
	member := core.Object(members[0].(map[string]any))
	assert.Len(t, member, 2)
	assert.Equal(t, "Group", member.Get("type"))
}

func TestApplyReplaceMembersLeavesAnExplicitImmutableValueAlone(t *testing.T) {
	item := core.Object{"members": []any{map[string]any{"value": "u-1", "type": "User"}}}

	require.NoError(t, apply(item, groupSchemas(), operation(patch.OpReplace, "members", `[{"value":"u-1","type":"Group"}]`)))

	members := item["members"].([]any)
	require.Len(t, members, 1)
	assert.Equal(t, "Group", members[0].(map[string]any)["type"])
}

func TestApplyReplaceMembersKeepsOmittedImmutableSubAttributesFromASingleObjectValue(t *testing.T) {
	item := core.Object{"members": []any{map[string]any{"value": "u-1", "type": "User"}}}

	require.NoError(t, apply(item, groupSchemas(), operation(patch.OpReplace, "members", `{"value":"u-1"}`)))

	members := item["members"].([]any)
	require.Len(t, members, 1)
	assert.Equal(t, "User", members[0].(map[string]any)["type"])
}

func TestApplyReplaceWithoutPathKeepsOmittedImmutableSubAttributes(t *testing.T) {
	item := core.Object{"members": []any{map[string]any{"value": "u-1", "type": "User"}}}

	require.NoError(t, apply(item, groupSchemas(), patch.Operation{Op: patch.OpReplace, Value: json.RawMessage(`{"members":[{"value":"u-1"}]}`)}))

	members := item["members"].([]any)
	require.Len(t, members, 1)
	assert.Equal(t, "User", members[0].(map[string]any)["type"])
}

// RFC 7644 Section 3.5.2: a client MUST NOT modify an attribute that has mutability "immutable".
func TestApplyReplaceKeepsOmittedImmutableSubAttributesWithoutAValueSubAttribute(t *testing.T) {
	schemas := []*core.Schema{
		(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("parts", core.TypeComplex).AsMultiValued().With(
				core.NewAttribute("serial", core.TypeString),
				core.NewAttribute("code", core.TypeString).AsImmutable(),
			),
		),
	}
	item := core.Object{"parts": []any{map[string]any{"serial": "s-1", "code": "A"}}}

	require.NoError(t, apply(item, schemas, operation(patch.OpReplace, "parts", `[{"serial":"s-1"}]`)))

	parts := item["parts"].([]any)
	require.Len(t, parts, 1)
	assert.Equal(t, "A", parts[0].(map[string]any)["code"])
}

// RFC 7643 Section 2.5: "null" for a multi-valued attribute clears it rather than storing an array holding null.
func TestApplyReplaceMultiValuedWithNullClearsIt(t *testing.T) {
	item := map[string]any{"emails": []any{map[string]any{"value": "a@b.com", "type": "work"}}}

	require.NoError(t, apply(item, userSchemas(), operation(patch.OpReplace, "emails", "null")))

	assert.Empty(t, item["emails"].([]any))
}

// RFC 7644 Section 3.4.2.2: DateTime comparison is chronological, not lexical.
func TestApplyValueFilterDateTimeComparesTemporally(t *testing.T) {
	schemas := []*core.Schema{
		(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("events", core.TypeComplex).AsMultiValued().With(
				core.NewAttribute("at", core.TypeDateTime),
				core.NewAttribute("label", core.TypeString),
			),
		),
	}
	item := map[string]any{"events": []any{
		map[string]any{"at": "2024-01-01T23:00:00-05:00", "label": "later by instant, earlier lexically"},
	}}

	require.NoError(t, apply(item, schemas, operation(patch.OpReplace, `events[at ge "2024-01-02T00:00:00Z"].label`, `"matched"`)))

	events := item["events"].([]any)
	assert.Equal(t, "matched", events[0].(map[string]any)["label"])
}

func TestApplyAddSubAttributeIntoAbsentMultiValuedIsInvalidPath(t *testing.T) {
	var scimErr *scimerrors.Error

	require.ErrorAs(t, apply(map[string]any{}, userSchemas(), operation(patch.OpAdd, "emails.type", `"work"`)), &scimErr)
	assert.Equal(t, scimerrors.InvalidPath, scimErr.ScimType)
}

// RFC 7643 Section 7: an assigned immutable attribute rejects an "add" that changes its value, even when the attribute is multi-valued.
func TestApplyAddRejectsANewValueOntoAnAssignedImmutableMultiValuedAttribute(t *testing.T) {
	item := core.Object{"tags": []any{"a"}}

	requireMutability(t, apply(item, tagsSchema(), operation(patch.OpAdd, "tags", `["b"]`)))
	assert.Equal(t, []any{"a"}, item["tags"])
}

// RFC 7644 Section 3.5.2.1: adding a value the target already contains makes no change, even when the attribute is immutable.
func TestApplyAddOfAnAlreadyPresentValueOntoAnImmutableMultiValuedAttributeIsANoOp(t *testing.T) {
	item := core.Object{"tags": []any{"a"}}

	require.NoError(t, apply(item, tagsSchema(), operation(patch.OpAdd, "tags", `["a"]`)))
	assert.Equal(t, []any{"a"}, item["tags"])
}

// RFC 7643 Section 2.5: an empty array is unassigned, so an immutable attribute may still receive its first value via "add".
func TestApplyAddFirstValueOntoEmptyImmutableMultiValuedAttributeSucceeds(t *testing.T) {
	item := core.Object{"tags": []any{}}

	require.NoError(t, apply(item, tagsSchema(), operation(patch.OpAdd, "tags", `["a"]`)))
	assert.Equal(t, []any{"a"}, item["tags"])
}

// RFC 7643 Section 7: a value-filter-matched multi-valued immutable sub-attribute also rejects an "add" that changes its value.
func TestApplyAddRejectsANewValueOntoAnImmutableMultiValuedSubAttributeMatchedByFilter(t *testing.T) {
	schemas := []*core.Schema{
		(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("parts", core.TypeComplex).AsMultiValued().With(
				core.NewAttribute("serial", core.TypeString),
				core.NewAttribute("codes", core.TypeString).AsMultiValued().AsImmutable(),
			),
		),
	}
	item := core.Object{"parts": []any{map[string]any{"serial": "s-1", "codes": []any{"A"}}}}

	requireMutability(t, apply(item, schemas, operation(patch.OpAdd, `parts[serial eq "s-1"].codes`, `["B"]`)))
}

// RFC 7644 Section 3.5.2.1: merging a value-filter-matched element must not duplicate a field the element already has.
func TestApplyAddMergedIntoAFilteredElementSkipsAnAlreadyPresentValue(t *testing.T) {
	schemas := []*core.Schema{
		(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("parts", core.TypeComplex).AsMultiValued().With(
				core.NewAttribute("serial", core.TypeString),
				core.NewAttribute("codes", core.TypeString).AsMultiValued(),
			),
		),
	}
	item := core.Object{"parts": []any{map[string]any{"serial": "s-1", "codes": []any{"A"}}}}

	require.NoError(t, apply(item, schemas, operation(patch.OpAdd, `parts[serial eq "s-1"]`, `{"codes":["A"]}`)))

	parts := item["parts"].([]any)
	assert.Equal(t, []any{"A"}, parts[0].(map[string]any)["codes"])
}

// RFC 7644 Section 3.5.2.1: adding an already-present value to a filtered sub-attribute must not duplicate it.
func TestApplyAddToAFilteredSubAttributeSkipsAnAlreadyPresentValue(t *testing.T) {
	schemas := []*core.Schema{
		(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("parts", core.TypeComplex).AsMultiValued().With(
				core.NewAttribute("serial", core.TypeString),
				core.NewAttribute("codes", core.TypeString).AsMultiValued(),
			),
		),
	}
	item := core.Object{"parts": []any{map[string]any{"serial": "s-1", "codes": []any{"A"}}}}

	require.NoError(t, apply(item, schemas, operation(patch.OpAdd, `parts[serial eq "s-1"].codes`, `["A"]`)))

	parts := item["parts"].([]any)
	assert.Equal(t, []any{"A"}, parts[0].(map[string]any)["codes"])
}

// RFC 7644 Section 3.5.2.1: if the target location already contains the value specified, no changes SHOULD be made, even when immutable.
func TestApplyAddOfAnAlreadyPresentValueToAnImmutableFilteredSubAttributeIsANoOp(t *testing.T) {
	schemas := []*core.Schema{
		(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("parts", core.TypeComplex).AsMultiValued().With(
				core.NewAttribute("serial", core.TypeString),
				core.NewAttribute("codes", core.TypeString).AsMultiValued().AsImmutable(),
			),
		),
	}
	item := core.Object{"parts": []any{map[string]any{"serial": "s-1", "codes": []any{"A"}}}}

	require.NoError(t, apply(item, schemas, operation(patch.OpAdd, `parts[serial eq "s-1"].codes`, `["A"]`)))

	parts := item["parts"].([]any)
	assert.Equal(t, []any{"A"}, parts[0].(map[string]any)["codes"])
}

// RFC 7643 Section 7: writing a sub-attribute also changes its immutable parent complex attribute once the parent is assigned.
func TestApplyReplaceRejectsAWriteThroughAnAssignedImmutableParent(t *testing.T) {
	item := core.Object{"badge": map[string]any{"number": "1"}}

	requireMutability(t, apply(item, badgeSchema(), operation(patch.OpReplace, "badge.number", `"2"`)))
	assert.Equal(t, "1", item["badge"].(map[string]any)["number"])
}

// RFC 7643 Section 7: an immutable attribute SHALL NOT be updated, so an unchanged write through its parent succeeds.
func TestApplyReplaceOfAnEqualValueThroughAnImmutableParentIsANoOp(t *testing.T) {
	item := core.Object{"badge": map[string]any{"number": "1"}}

	require.NoError(t, apply(item, badgeSchema(), operation(patch.OpReplace, "badge.number", `"1"`)))
	assert.Equal(t, "1", item["badge"].(map[string]any)["number"])
}

// RFC 7643 Section 7: removing a sub-attribute also changes its immutable parent complex attribute once the parent is assigned.
func TestApplyRemoveRejectsARemovalThroughAnAssignedImmutableParent(t *testing.T) {
	item := core.Object{"badge": map[string]any{"number": "1"}}

	requireMutability(t, apply(item, badgeSchema(), operation(patch.OpRemove, "badge.number", "")))
	assert.Equal(t, "1", item["badge"].(map[string]any)["number"])
}

// RFC 7644 Section 3.5.2.1: adding an already-present value to a mutable sub-attribute makes no change, even through an immutable parent.
func TestApplyAddOfAnAlreadyPresentValueToAMutableSubAttributeThroughAnImmutableParentIsANoOp(t *testing.T) {
	item := core.Object{"badge": map[string]any{"codes": []any{"A"}}}

	require.NoError(t, apply(item, badgeCodesSchema(), operation(patch.OpAdd, "badge.codes", `["A"]`)))
	assert.Equal(t, []any{"A"}, item["badge"].(map[string]any)["codes"])
}

// RFC 7643 Section 7: an assigned immutable attribute SHALL NOT be updated, including by removal.
func TestApplyRemoveRejectsAnAssignedTopLevelImmutableAttribute(t *testing.T) {
	schemas := []*core.Schema{
		(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("employeeNumber", core.TypeString).AsImmutable(),
		),
	}
	item := core.Object{"employeeNumber": "1"}

	requireMutability(t, apply(item, schemas, operation(patch.OpRemove, "employeeNumber", "")))
	assert.Equal(t, "1", item["employeeNumber"])
}

func badgeSchema() []*core.Schema {
	return []*core.Schema{
		(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("badge", core.TypeComplex).AsImmutable().With(
				core.NewAttribute("number", core.TypeString),
			),
		),
	}
}

func badgeCodesSchema() []*core.Schema {
	return []*core.Schema{
		(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("badge", core.TypeComplex).AsImmutable().With(
				core.NewAttribute("codes", core.TypeString).AsMultiValued(),
			),
		),
	}
}

func apply(resource core.Object, schemas []*core.Schema, ops ...patch.Operation) error {
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

func tagsSchema() []*core.Schema {
	return []*core.Schema{
		(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("tags", core.TypeString).AsMultiValued().AsImmutable(),
		),
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

func groupSchemas() []*core.Schema {
	return []*core.Schema{
		(&core.Schema{ID: core.SchemaGroup, Name: "Group"}).With(core.GroupAttributes()...),
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
