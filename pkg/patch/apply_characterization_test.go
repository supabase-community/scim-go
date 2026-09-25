package patch_test

import (
	"encoding/json"
	"maps"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

func TestApplyValuePathComparisonMatrix(t *testing.T) {
	cases := []struct {
		name    string
		element map[string]any
		filter  string
		match   bool
	}{
		{"string eq", map[string]any{"value": "a@b.com"}, `value eq "a@b.com"`, true},
		{"string eq folds case", map[string]any{"value": "a@b.com"}, `value eq "A@B.COM"`, true},
		{"string eq miss", map[string]any{"value": "a@b.com"}, `value eq "x@y.com"`, false},
		{"string ne", map[string]any{"value": "a@b.com"}, `value ne "x@y.com"`, true},
		{"string co", map[string]any{"value": "a@b.com"}, `value co "b.c"`, true},
		{"string sw", map[string]any{"value": "a@b.com"}, `value sw "a@"`, true},
		{"string ew", map[string]any{"value": "a@b.com"}, `value ew ".com"`, true},
		{"string gt", map[string]any{"value": "b"}, `value gt "a"`, true},
		{"string lt", map[string]any{"value": "a"}, `value lt "b"`, true},
		{"string ge", map[string]any{"value": "a"}, `value ge "a"`, true},
		{"string le", map[string]any{"value": "a"}, `value le "a"`, true},

		{"number eq json.Number", map[string]any{"score": json.Number("5")}, `score eq 5`, true},
		{"number eq float64", map[string]any{"score": float64(5)}, `score eq 5`, true},
		{"number eq int64", map[string]any{"score": int64(5)}, `score eq 5`, true},
		{"number ne", map[string]any{"score": json.Number("5")}, `score ne 4`, true},
		{"number gt", map[string]any{"score": json.Number("5")}, `score gt 4`, true},
		{"number lt", map[string]any{"score": json.Number("5")}, `score lt 6`, true},
		{"number ge", map[string]any{"score": json.Number("5")}, `score ge 5`, true},
		{"number le", map[string]any{"score": json.Number("5")}, `score le 5`, true},
		{"number miss", map[string]any{"score": json.Number("5")}, `score gt 9`, false},

		{"bool eq", map[string]any{"active": true}, `active eq true`, true},
		{"bool ne", map[string]any{"active": true}, `active ne false`, true},
		{"bool miss", map[string]any{"active": true}, `active eq false`, false},

		{"presence", map[string]any{"value": "a@b.com"}, `value pr`, true},
		{"presence miss", map[string]any{"score": json.Number("5")}, `value pr`, false},

		{"native int matches", map[string]any{"score": 5}, `score eq 5`, true},
		{"nil value never matches", map[string]any{"value": nil}, `value eq "x"`, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := map[string]any{"emails": []any{maps.Clone(tc.element)}}
			op := operation(patch.OpReplace, `emails[`+tc.filter+`]`, `{"tag":"hit"}`)

			err := apply(item, comparisonSchemas(), op)

			if !tc.match {
				var scimErr *scimerrors.Error
				require.ErrorAs(t, err, &scimErr)
				assert.Equal(t, scimerrors.NoTarget, scimErr.ScimType)
				return
			}
			require.NoError(t, err)
			element := item["emails"].([]any)[0].(map[string]any)
			assert.Equal(t, "hit", element["tag"])
		})
	}
}

func TestApplyRemoveSubAttribute(t *testing.T) {
	item := map[string]any{"name": map[string]any{"familyName": "Jensen", "givenName": "Barbara"}}

	require.NoError(t, apply(item, nil, operation(patch.OpRemove, "name.familyName", "")))

	name := item["name"].(map[string]any)
	_, present := name["familyName"]
	assert.False(t, present)
	assert.Equal(t, "Barbara", name["givenName"])
}

func TestApplyResolvesMixedCaseTopLevelKey(t *testing.T) {
	item := map[string]any{"UserName": "old"}

	require.NoError(t, apply(item, nil, operation(patch.OpReplace, "userName", `"new"`)))

	assert.Equal(t, "new", item["UserName"])
	_, duplicated := item["userName"]
	assert.False(t, duplicated)
}

// RFC 7643 2.1 - attribute names are case insensitive; folding collisions resolve to the lowest-ordered key.
func TestApplyResolvesDuplicateFoldingKeysDeterministically(t *testing.T) {
	item := map[string]any{"UserName": "upper", "username": "lower"}

	require.NoError(t, apply(item, nil, operation(patch.OpReplace, "userName", `"new"`)))

	assert.Equal(t, "new", item["UserName"])
	assert.Equal(t, "lower", item["username"])
}

// RFC 7644 Section 3.5.2: an operation incompatible with an attribute's mutability SHALL return an error.
func TestApplyNoPathMergeRejectsReadOnly(t *testing.T) {
	item := map[string]any{}

	var scimErr *scimerrors.Error
	require.ErrorAs(t, apply(item, userSchemas(), patch.Operation{
		Op:    patch.OpReplace,
		Value: json.RawMessage(`{"displayName":"Babs","groups":[{"value":"g1"}]}`),
	}), &scimErr)
	assert.Equal(t, scimerrors.Mutability, scimErr.ScimType)
	assert.Empty(t, item)
}

func TestApplySubAttributeOnNonComplexRejected(t *testing.T) {
	item := map[string]any{"userName": "bob"}

	var scimErr *scimerrors.Error
	require.ErrorAs(t, apply(item, nil, operation(patch.OpReplace, "userName.foo", `"x"`)), &scimErr)
	assert.Equal(t, scimerrors.InvalidPath, scimErr.ScimType)
}

func TestApplyRejectsMalformedValueJSON(t *testing.T) {
	var scimErr *scimerrors.Error
	require.ErrorAs(t, apply(map[string]any{}, nil, patch.Operation{
		Op:    patch.OpReplace,
		Path:  "userName",
		Value: json.RawMessage(`{bad`),
	}), &scimErr)
	assert.Equal(t, scimerrors.InvalidValue, scimErr.ScimType)
}

func TestApplyRejectsInvalidPath(t *testing.T) {
	var scimErr *scimerrors.Error
	require.ErrorAs(t, apply(map[string]any{}, nil, operation(patch.OpRemove, `emails[`, "")), &scimErr)
	assert.Equal(t, scimerrors.InvalidPath, scimErr.ScimType)
}

func TestApplyValuePathReadOnlySubAttributeRejected(t *testing.T) {
	schemas := []*core.Schema{
		(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("emails", core.TypeComplex).AsMultiValued().With(
				core.NewAttribute("value", core.TypeString),
				core.NewAttribute("primary", core.TypeBoolean).AsReadOnly(),
			),
		),
	}
	item := map[string]any{"emails": []any{map[string]any{"value": "a@b.com"}}}

	var scimErr *scimerrors.Error
	require.ErrorAs(t, apply(item, schemas, operation(patch.OpReplace, `emails[value eq "a@b.com"].primary`, `true`)), &scimErr)
	assert.Equal(t, scimerrors.Mutability, scimErr.ScimType)

	member := item["emails"].([]any)[0].(map[string]any)
	assert.NotContains(t, member, "primary")
}

// RFC 7644 Section 3.12, Table 9: a PATCH path filter fails like a search filter does.
func TestApplyValuePathRejectsInvalidComparisons(t *testing.T) {
	cases := []struct {
		filter   string
		scimType scimerrors.ErrorType
	}{
		{`value gt 5`, scimerrors.InvalidValue},
		{`active gt false`, scimerrors.InvalidFilter},
		{`bogus eq "x"`, scimerrors.InvalidFilter},
	}
	for _, tc := range cases {
		t.Run(tc.filter, func(t *testing.T) {
			item := map[string]any{"emails": []any{map[string]any{"value": "a@b.com", "active": true}}}

			var scimErr *scimerrors.Error
			require.ErrorAs(t, apply(item, comparisonSchemas(), operation(patch.OpReplace, `emails[`+tc.filter+`]`, `{"tag":"hit"}`)), &scimErr)
			assert.Equal(t, tc.scimType, scimErr.ScimType)
		})
	}
}

func comparisonSchemas() []*core.Schema {
	return []*core.Schema{
		(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("emails", core.TypeComplex).AsMultiValued().With(
				core.NewAttribute("value", core.TypeString),
				core.NewAttribute("score", core.TypeDecimal),
				core.NewAttribute("active", core.TypeBoolean),
			),
		),
	}
}
