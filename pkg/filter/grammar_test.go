package filter

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGrammarAttributePath(t *testing.T) {
	tt := []struct {
		name  string
		input string
		want  string
	}{
		{"simple", `userName eq "x"`, "userName"},
		{"dotted sub-attribute", `name.familyName eq "x"`, "name.familyName"},
		{"schema-URI-prefixed", `urn:ietf:params:scim:schemas:core:2.0:User:userName eq "x"`, "urn:ietf:params:scim:schemas:core:2.0:User:userName"},
		{"schema-URI-prefixed with sub-attribute", `urn:ietf:params:scim:schemas:core:2.0:User:name.familyName eq "x"`, "urn:ietf:params:scim:schemas:core:2.0:User:name.familyName"},
	}
	g := &Grammar{}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			node, err := g.Parse(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.want, node.Attribute())
		})
	}
}

func TestGrammarLeafComparison(t *testing.T) {
	g := &Grammar{}
	node, err := g.Parse(`userName eq "bjensen"`)

	require.NoError(t, err)
	require.NotNil(t, node)
	assert.Equal(t, "userName", node.Attribute())
	assert.Equal(t, "eq", node.Operator())
	assert.Equal(t, "bjensen", node.Value())
}

func TestGrammarComparisonOperatorsAndLiterals(t *testing.T) {
	tt := []struct {
		input     string
		attribute string
		operator  string
		value     any
	}{
		{`userName eq "bjensen"`, "userName", "eq", "bjensen"},
		{`userName ne "bjensen"`, "userName", "ne", "bjensen"},
		{`userName co "jen"`, "userName", "co", "jen"},
		{`userName sw "bjen"`, "userName", "sw", "bjen"},
		{`userName ew "sen"`, "userName", "ew", "sen"},
		{`age gt 21`, "age", "gt", 21.0},
		{`age ge 21`, "age", "ge", 21.0},
		{`age lt 21`, "age", "lt", 21.0},
		{`age le 21`, "age", "le", 21.0},
		{`salary ge 1.5e4`, "salary", "ge", 1.5e4},
		{`salary ge -3.5`, "salary", "ge", -3.5},
		{`active eq true`, "active", "eq", true},
		{`active eq false`, "active", "eq", false},
		{`nickName eq null`, "nickName", "eq", nil},
	}
	g := &Grammar{}
	for _, tc := range tt {
		t.Run(tc.input, func(t *testing.T) {
			node, err := g.Parse(tc.input)

			require.NoError(t, err)
			require.NotNil(t, node)
			assert.Equal(t, tc.attribute, node.Attribute())
			assert.Equal(t, tc.operator, node.Operator())
			assert.Equal(t, tc.value, node.Value())
		})
	}
}

func TestGrammarPresence(t *testing.T) {
	g := &Grammar{}
	node, err := g.Parse("userName pr")

	require.NoError(t, err)
	require.NotNil(t, node)
	assert.Equal(t, "userName", node.Attribute())
	assert.Equal(t, "pr", node.Operator())
	assert.Nil(t, node.Value())
}

func TestGrammarLogicalExpression(t *testing.T) {
	g := &Grammar{}

	t.Run("and", func(t *testing.T) {
		node, err := g.Parse(`userName eq "bjensen" and active eq true`)

		require.NoError(t, err)
		assert.Equal(t, "and", node.Operator())
		assert.Equal(t, "userName", node.Left().Attribute())
		assert.Equal(t, "active", node.Right().Attribute())
	})

	t.Run("or", func(t *testing.T) {
		node, err := g.Parse(`userName eq "bjensen" or userName eq "jsmith"`)

		require.NoError(t, err)
		assert.Equal(t, "or", node.Operator())
		assert.Equal(t, "bjensen", node.Left().Value())
		assert.Equal(t, "jsmith", node.Right().Value())
	})

	t.Run("chained and", func(t *testing.T) {
		node, err := g.Parse(`a eq "1" and b eq "2" and c eq "3"`)

		require.NoError(t, err)
		assert.Equal(t, "and", node.Operator())
		assert.Equal(t, "a", node.Left().Attribute())
		assert.Equal(t, "and", node.Right().Operator())
		assert.Equal(t, "b", node.Right().Left().Attribute())
		assert.Equal(t, "c", node.Right().Right().Attribute())
	})

	t.Run("and binds tighter than or, regardless of textual order", func(t *testing.T) {
		node, err := g.Parse(`a eq "1" and b eq "2" or c eq "3"`)

		require.NoError(t, err)
		require.Equal(t, "or", node.Operator(), "expected (a and b) or c, i.e. root operator is or")
		assert.Equal(t, "and", node.Left().Operator())
		assert.Equal(t, "a", node.Left().Left().Attribute())
		assert.Equal(t, "b", node.Left().Right().Attribute())
		assert.Equal(t, "c", node.Right().Attribute())
	})

	t.Run("or on the left, and on the right, and still binds tighter", func(t *testing.T) {
		node, err := g.Parse(`a eq "1" or b eq "2" and c eq "3"`)

		require.NoError(t, err)
		require.Equal(t, "or", node.Operator(), "expected a or (b and c)")
		assert.Equal(t, "a", node.Left().Attribute())
		assert.Equal(t, "and", node.Right().Operator())
		assert.Equal(t, "b", node.Right().Left().Attribute())
		assert.Equal(t, "c", node.Right().Right().Attribute())
	})
}

func TestGrammarParensAndNot(t *testing.T) {
	g := &Grammar{}

	t.Run("not wrapping a parenthesized leaf", func(t *testing.T) {
		node, err := g.Parse(`not (userName eq "bjensen")`)

		require.NoError(t, err)
		assert.True(t, node.Not())
		assert.Equal(t, "userName", node.Operand().Attribute())
		assert.Equal(t, "eq", node.Operand().Operator())
	})

	t.Run("nested not stays distinct at each level", func(t *testing.T) {
		node, err := g.Parse(`not (not (userName pr))`)

		require.NoError(t, err)
		require.True(t, node.Not())
		require.True(t, node.Operand().Not())
		assert.Equal(t, "userName", node.Operand().Operand().Attribute())
		assert.Equal(t, "pr", node.Operand().Operand().Operator())
	})

	t.Run("parens override precedence", func(t *testing.T) {
		node, err := g.Parse(`(a eq "1" or b eq "2") and c eq "3"`)

		require.NoError(t, err)
		require.Equal(t, "and", node.Operator())
		assert.Equal(t, "or", node.Left().Operator())
		assert.Equal(t, "c", node.Right().Attribute())
	})

	t.Run("not without parens is rejected", func(t *testing.T) {
		_, err := g.Parse(`not userName pr`)

		assert.Error(t, err)
	})
}

func TestGrammarValuePath(t *testing.T) {
	g := &Grammar{}

	t.Run("simple", func(t *testing.T) {
		node, err := g.Parse(`emails[type eq "work"]`)

		require.NoError(t, err)
		require.True(t, node.HasPath())
		assert.Equal(t, "emails", node.Path())
		assert.Equal(t, "", node.SubAttribute())
		assert.Equal(t, "type", node.ValueFilter().Attribute())
		assert.Equal(t, "work", node.ValueFilter().Value())
	})

	t.Run("with a trailing sub-attribute", func(t *testing.T) {
		node, err := g.Parse(`emails[type eq "work"].value`)

		require.NoError(t, err)
		assert.Equal(t, "emails", node.Path())
		assert.Equal(t, "value", node.SubAttribute())
	})

	t.Run("value filter supports and/or", func(t *testing.T) {
		node, err := g.Parse(`emails[type eq "work" and primary eq true]`)

		require.NoError(t, err)
		vf := node.ValueFilter()
		assert.Equal(t, "and", vf.Operator())
		assert.Equal(t, "type", vf.Left().Attribute())
		assert.Equal(t, "primary", vf.Right().Attribute())
	})
}

func TestGrammarRejectsTrailingGarbage(t *testing.T) {
	g := &Grammar{}
	_, err := g.Parse(`userName eq "bjensen" garbage`)

	assert.Error(t, err)
}

func TestParseRejectsOversizedInput(t *testing.T) {
	g := &Grammar{MaxInputBytes: 16}

	t.Run("input over the cap is rejected before parsing", func(t *testing.T) {
		_, err := g.Parse(`userName eq "bjensen"`) // 21 bytes > 16
		require.ErrorIs(t, err, ErrInputTooLarge)
		assert.NotErrorIs(t, err, ErrInvalidFilter)
	})

	t.Run("input within the cap still parses", func(t *testing.T) {
		node, err := g.Parse(`a eq "1"`) // 8 bytes <= 16
		require.NoError(t, err)
		assert.Equal(t, "a", node.Attribute())
	})
}

func TestParseDefaultIsUnbounded(t *testing.T) {
	g := New() // MaxInputBytes defaults to 0
	const depth = 4000
	input := strings.Repeat("(", depth) + `a eq "1"` + strings.Repeat(")", depth)

	node, err := g.Parse(input)

	require.NoError(t, err)
	assert.Equal(t, "a", node.Attribute())
}

func TestParseErrorReportsPosition(t *testing.T) {
	g := &Grammar{}
	input := `userName eq "bjensen" garbage`
	_, err := g.Parse(input)

	require.ErrorIs(t, err, ErrInvalidFilter)
	var perr *ParseError
	require.ErrorAs(t, err, &perr)
	assert.Equal(t, input, perr.Input)
	assert.Equal(t, len("userName eq \"bjensen\" "), perr.Position)
}

func TestGrammarTrailingWhitespaceIsTolerated(t *testing.T) {
	g := &Grammar{}
	node, err := g.Parse(`userName eq "bjensen"   `)

	require.NoError(t, err)
	assert.Equal(t, "userName", node.Attribute())
}

func TestGrammarDeepNestingIsLinear(t *testing.T) {
	g := &Grammar{}
	const depth = 2000
	input := strings.Repeat("(", depth) + `userName eq "bjensen"` + strings.Repeat(")", depth)

	node, err := g.Parse(input)

	require.NoError(t, err)
	assert.Equal(t, "userName", node.Attribute())
	assert.Equal(t, "eq", node.Operator())
}

func TestGrammarOperatorsAreCaseInsensitive(t *testing.T) {
	// RFC 7644 3.4.2.2: attribute operators and logical keywords are case insensitive.
	g := &Grammar{}

	t.Run("comparison operator", func(t *testing.T) {
		node, err := g.Parse(`userName EQ "bjensen"`)
		require.NoError(t, err)
		assert.Equal(t, "eq", node.Operator())
	})

	t.Run("logical keyword", func(t *testing.T) {
		node, err := g.Parse(`userName eq "a" AND active pr`)
		require.NoError(t, err)
		assert.Equal(t, "and", node.Operator())
	})

	t.Run("presence", func(t *testing.T) {
		node, err := g.Parse(`userName PR`)
		require.NoError(t, err)
		assert.Equal(t, "pr", node.Operator())
	})
}

func TestGrammarJSONLiteralsAreCaseSensitive(t *testing.T) {
	// compValue uses JSON rules (RFC 7159); true/false/null are lowercase only.
	g := &Grammar{}

	for _, good := range []string{`active eq true`, `active eq false`, `nickName eq null`} {
		t.Run("accepts "+good, func(t *testing.T) {
			_, err := g.Parse(good)
			assert.NoError(t, err)
		})
	}
	for _, bad := range []string{`active eq TRUE`, `active eq False`, `nickName eq Null`} {
		t.Run("rejects "+bad, func(t *testing.T) {
			_, err := g.Parse(bad)
			assert.Error(t, err)
		})
	}
}

func TestGrammarStringEscapesAreDecoded(t *testing.T) {
	// compValue strings follow JSON rules, so escapes must be decoded.
	tt := []struct {
		input string
		want  string
	}{
		{`userName eq "a\"b"`, `a"b`},
		{`userName eq "a\\b"`, `a\b`},
		{`userName eq "tab\tend"`, "tab\tend"},
		{`userName eq "A"`, "A"},
	}
	g := &Grammar{}
	for _, tc := range tt {
		t.Run(tc.input, func(t *testing.T) {
			node, err := g.Parse(tc.input)
			require.NoError(t, err)
			assert.Equal(t, tc.want, node.Value())
		})
	}
}

func TestGrammarRejectsMalformed(t *testing.T) {
	tt := []struct {
		name  string
		input string
	}{
		{"dangling and", `userName eq "a" and`},
		{"dangling or", `userName eq "a" or`},
		{"invalid string escape", `userName eq "\x"`},
		{"number out of range", `age gt 1e400`},
		{"unclosed paren", `not (userName eq "a"`},
		{"unopened paren", `userName eq "a")`},
	}
	g := &Grammar{}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			_, err := g.Parse(tc.input)
			assert.ErrorIs(t, err, ErrInvalidFilter)
		})
	}
}

func TestGrammarValueFilterRejectsNestedValuePath(t *testing.T) {
	// valFilter = attrExp / logExp / *1"not" "(" valFilter ")" -- no nested valuePath.
	g := &Grammar{}

	t.Run("nested value path is rejected", func(t *testing.T) {
		_, err := g.Parse(`emails[members[type eq "work"]]`)
		assert.Error(t, err)
	})

	t.Run("not-group inside brackets is accepted", func(t *testing.T) {
		node, err := g.Parse(`emails[not (type eq "work")]`)
		require.NoError(t, err)
		require.True(t, node.HasPath())
		assert.True(t, node.ValueFilter().Not())
	})
}
