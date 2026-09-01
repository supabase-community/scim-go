package protocol

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type filterUser struct {
	UserName string         `json:"userName"`
	Active   bool           `json:"active"`
	Emails   []filterEmail  `json:"emails,omitempty"`
	Meta     filterUserMeta `json:"meta"`
}

type filterEmail struct {
	Value string `json:"value"`
	Type  string `json:"type"`
}

type filterUserMeta struct {
	Created string `json:"created,omitempty"`
}

func TestParseFilter(t *testing.T) {
	t.Run("an empty expression means no filter", func(t *testing.T) {
		filter, err := ParseFilter("")

		require.NoError(t, err)
		require.Nil(t, filter)
	})

	t.Run("parses each comparison operator", func(t *testing.T) {
		for _, tc := range []struct {
			op FilterOp
			in string
		}{
			{OpEqual, "eq"}, {OpNotEqual, "ne"}, {OpContains, "co"},
			{OpStartsWith, "sw"}, {OpEndsWith, "ew"},
			{OpGreaterThan, "gt"}, {OpGreaterEqual, "ge"},
			{OpLessThan, "lt"}, {OpLessEqual, "le"},
		} {
			t.Run(tc.in, func(t *testing.T) {
				filter, err := ParseFilter(`userName ` + tc.in + ` "bjensen"`)

				require.NoError(t, err)
				require.Equal(t, &AttrExpr{
					Path:  AttrPath{Attribute: "userName"},
					Op:    tc.op,
					Value: "bjensen",
				}, filter)
			})
		}
	})

	t.Run("parses presence", func(t *testing.T) {
		filter, err := ParseFilter("active pr")

		require.NoError(t, err)
		require.Equal(t, &AttrExpr{Path: AttrPath{Attribute: "active"}, Op: OpPresent}, filter)
	})

	t.Run("parses and/or/not with correct precedence", func(t *testing.T) {
		filter, err := ParseFilter(`userName eq "a" and active eq true or not (userName eq "b")`)

		require.NoError(t, err)
		require.Equal(t, &LogicalExpr{
			Op: "or",
			Left: &LogicalExpr{
				Op:   "and",
				Left: &AttrExpr{Path: AttrPath{Attribute: "userName"}, Op: OpEqual, Value: "a"},
				Right: &AttrExpr{
					Path: AttrPath{Attribute: "active"}, Op: OpEqual, Value: true,
				},
			},
			Right: &NotExpr{
				Expr: &AttrExpr{Path: AttrPath{Attribute: "userName"}, Op: OpEqual, Value: "b"},
			},
		}, filter)
	})

	t.Run("parses grouping", func(t *testing.T) {
		filter, err := ParseFilter(`(userName eq "a" or userName eq "b") and active eq true`)

		require.NoError(t, err)
		require.Equal(t, &LogicalExpr{
			Op: "and",
			Left: &LogicalExpr{
				Op:    "or",
				Left:  &AttrExpr{Path: AttrPath{Attribute: "userName"}, Op: OpEqual, Value: "a"},
				Right: &AttrExpr{Path: AttrPath{Attribute: "userName"}, Op: OpEqual, Value: "b"},
			},
			Right: &AttrExpr{Path: AttrPath{Attribute: "active"}, Op: OpEqual, Value: true},
		}, filter)
	})

	t.Run("parses a dotted sub-attribute", func(t *testing.T) {
		filter, err := ParseFilter(`emails.type eq "work"`)

		require.NoError(t, err)
		require.Equal(t, &AttrExpr{
			Path: AttrPath{Attribute: "emails", SubAttr: "type"}, Op: OpEqual, Value: "work",
		}, filter)
	})

	t.Run("parses a valuePath filter", func(t *testing.T) {
		filter, err := ParseFilter(`emails[type eq "work" and value co "@example.com"]`)

		require.NoError(t, err)
		require.Equal(t, &ValuePathExpr{
			Attribute: "emails",
			Sub: &LogicalExpr{
				Op:   "and",
				Left: &AttrExpr{Path: AttrPath{Attribute: "type"}, Op: OpEqual, Value: "work"},
				Right: &AttrExpr{
					Path: AttrPath{Attribute: "value"}, Op: OpContains, Value: "@example.com",
				},
			},
		}, filter)
	})

	t.Run("rejects a nested valuePath inside a valuePath", func(t *testing.T) {
		_, err := ParseFilter(`emails[type eq "work" and other[x eq 1]]`)

		require.Error(t, err)
	})

	t.Run("rejects malformed input", func(t *testing.T) {
		for _, expr := range []string{
			`userName eq`,
			`userName xx "a"`,
			`(userName eq "a"`,
			`userName eq "a" and`,
			`not userName eq "a"`,
		} {
			_, err := ParseFilter(expr)
			require.Error(t, err)
		}
	})
}

func TestMatches(t *testing.T) {
	bjensen := filterUser{
		UserName: "bjensen",
		Active:   true,
		Emails: []filterEmail{
			{Value: "b@work.example.com", Type: "work"},
			{Value: "b@home.example.com", Type: "home"},
		},
		Meta: filterUserMeta{Created: "2024-01-01T00:00:00Z"},
	}

	t.Run("a nil filter matches everything", func(t *testing.T) {
		ok, err := Matches(nil, bjensen)

		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("matches a simple equality case-insensitively", func(t *testing.T) {
		filter, err := ParseFilter(`userName eq "BJENSEN"`)
		require.NoError(t, err)

		ok, err := Matches(filter, bjensen)

		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("matches presence only when the value is non-empty", func(t *testing.T) {
		present, err := ParseFilter("userName pr")
		require.NoError(t, err)

		ok, err := Matches(present, bjensen)
		require.NoError(t, err)
		assert.True(t, ok)

		ok, err = Matches(present, filterUser{})
		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("matches a dotted sub-attribute against any element", func(t *testing.T) {
		filter, err := ParseFilter(`emails.type eq "home"`)
		require.NoError(t, err)

		ok, err := Matches(filter, bjensen)

		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("valuePath requires both conditions on the same element", func(t *testing.T) {
		sameElement, err := ParseFilter(`emails[type eq "work" and value co "@work.example.com"]`)
		require.NoError(t, err)
		ok, err := Matches(sameElement, bjensen)
		require.NoError(t, err)
		assert.True(t, ok, "expected a single element to satisfy both conditions")

		crossElement, err := ParseFilter(`emails[type eq "home" and value co "@work.example.com"]`)
		require.NoError(t, err)
		ok, err = Matches(crossElement, bjensen)
		require.NoError(t, err)
		assert.False(t, ok, "no single element has type=home and a @work.example.com address")

		dottedAnd, err := ParseFilter(`emails.type eq "home" and emails.value co "@work.example.com"`)
		require.NoError(t, err)
		ok, err = Matches(dottedAnd, bjensen)
		require.NoError(t, err)
		assert.True(t, ok, "dotted AND matches across different elements, unlike valuePath")
	})

	t.Run("matches ordered comparisons on timestamps", func(t *testing.T) {
		filter, err := ParseFilter(`meta.created ge "2023-01-01T00:00:00Z"`)
		require.NoError(t, err)

		ok, err := Matches(filter, bjensen)

		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("not negates its inner filter", func(t *testing.T) {
		filter, err := ParseFilter(`not (userName eq "someone-else")`)
		require.NoError(t, err)

		ok, err := Matches(filter, bjensen)

		require.NoError(t, err)
		assert.True(t, ok)
	})
}
