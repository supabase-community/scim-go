package protocol

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
)

func TestFilter(t *testing.T) {
	schema := (&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
		core.NewAttribute("userName", core.TypeString, ""),
		core.NewAttribute("active", core.TypeBoolean, ""),
		core.NewAttribute("age", core.TypeInteger, ""),
		core.NewAttribute("emails", core.TypeComplex, "").AsMultiValued().With(
			core.NewAttribute("type", core.TypeString, ""),
			core.NewAttribute("value", core.TypeString, ""),
		),
	)
	schemas := []*core.Schema{schema}

	t.Run("binds the coerced value as a parameter", func(t *testing.T) {
		out, err := Filter[clause](schemas, `active eq true`, sqlEvaluator{})
		require.NoError(t, err)
		assert.Equal(t, "active = ?", out.sql)
		assert.Equal(t, []any{true}, out.args)
	})

	t.Run("preserves the dotted key for a common sub-attribute", func(t *testing.T) {
		out, err := Filter[clause](schemas, `meta.lastModified gt "2020-01-01T00:00:00Z"`, sqlEvaluator{})
		require.NoError(t, err)
		assert.Equal(t, "meta.lastmodified > ?", out.sql)
		assert.Len(t, out.args, 1)
	})

	t.Run("composes and, or, not", func(t *testing.T) {
		out, err := Filter[clause](schemas, `userName eq "bob" and not (active eq false)`, sqlEvaluator{})
		require.NoError(t, err)
		assert.Equal(t, "(username = ? AND NOT (active = ?))", out.sql)
		assert.Equal(t, []any{"bob", false}, out.args)
	})

	t.Run("resolves presence", func(t *testing.T) {
		out, err := Filter[clause](schemas, `userName pr`, sqlEvaluator{})
		require.NoError(t, err)
		assert.Equal(t, "username IS NOT NULL", out.sql)
		assert.Empty(t, out.args)
	})

	t.Run("scopes a value path to the element sub-attributes", func(t *testing.T) {
		out, err := Filter[clause](schemas, `emails[type eq "work"]`, sqlEvaluator{})
		require.NoError(t, err)
		assert.Equal(t, "EXISTS(emails: type = ?)", out.sql)
		assert.Equal(t, []any{"work"}, out.args)
	})

	t.Run("composes a value path with a sibling comparison", func(t *testing.T) {
		out, err := Filter[clause](schemas, `emails[type eq "work"] and userName eq "bob"`, sqlEvaluator{})
		require.NoError(t, err)
		assert.Equal(t, "(EXISTS(emails: type = ?) AND username = ?)", out.sql)
		assert.Equal(t, []any{"work", "bob"}, out.args)
	})

	t.Run("resolves an extension attribute by schema URI", func(t *testing.T) {
		enterprise := (&core.Schema{ID: core.SchemaEnterpriseUser, Name: "EnterpriseUser"}).With(
			core.NewAttribute("department", core.TypeString, ""),
		)
		text := fmt.Sprintf("%s:department eq \"eng\"", core.SchemaEnterpriseUser)
		out, err := Filter[clause]([]*core.Schema{schema, enterprise}, text, sqlEvaluator{})
		require.NoError(t, err)
		assert.Equal(t, "department = ?", out.sql)
		assert.Equal(t, []any{"eng"}, out.args)
	})

	t.Run("renders gt, ge, lt, le comparisons", func(t *testing.T) {
		cases := map[string]string{
			`userName gt "a"`: "username > ?",
			`userName ge "a"`: "username >= ?",
			`userName lt "z"`: "username < ?",
			`userName le "z"`: "username <= ?",
		}
		for text, want := range cases {
			out, err := Filter[clause](schemas, text, sqlEvaluator{})
			require.NoError(t, err)
			assert.Equal(t, want, out.sql)
			assert.Len(t, out.args, 1)
		}
	})

	t.Run("composes or", func(t *testing.T) {
		out, err := Filter[clause](schemas, `userName eq "bob" or active eq true`, sqlEvaluator{})
		require.NoError(t, err)
		assert.Equal(t, "(username = ? OR active = ?)", out.sql)
		assert.Equal(t, []any{"bob", true}, out.args)
	})

	t.Run("renders an integer comparison", func(t *testing.T) {
		out, err := Filter[clause](schemas, `age gt 21`, sqlEvaluator{})
		require.NoError(t, err)
		assert.Equal(t, "age > ?", out.sql)
		assert.Equal(t, []any{int64(21)}, out.args)
	})

	t.Run("preserves an integer beyond float64 precision", func(t *testing.T) {
		out, err := Filter[clause](schemas, `age eq 9007199254740993`, sqlEvaluator{})
		require.NoError(t, err)
		assert.Equal(t, "age = ?", out.sql)
		assert.Equal(t, []any{int64(9007199254740993)}, out.args)
	})

	// RFC 7643 Section 2.3.4: an integer MUST NOT contain fractional or exponent parts.
	t.Run("rejects a fractional or exponent integer literal", func(t *testing.T) {
		for _, text := range []string{`age eq 21.0`, `age eq 2e3`, `age eq 21.5`} {
			_, err := Filter[clause](schemas, text, sqlEvaluator{})
			require.ErrorIs(t, err, ErrInvalidValue(""), text)
		}
	})

	t.Run("renders ne, co, sw, ew comparisons", func(t *testing.T) {
		cases := map[string]string{
			`userName ne "bob"`: "username <> ?",
			`userName co "ob"`:  "username co ?",
			`userName sw "bo"`:  "username sw ?",
			`userName ew "ob"`:  "username ew ?",
		}
		for text, want := range cases {
			out, err := Filter[clause](schemas, text, sqlEvaluator{})
			require.NoError(t, err, text)
			assert.Equal(t, want, out.sql)
			assert.Len(t, out.args, 1)
		}
	})

	// RFC 7644 Section 3.4.2.2: co, sw, ew apply only to string or reference attributes.
	t.Run("rejects a substring operator on a non-string attribute", func(t *testing.T) {
		_, err := Filter[clause](schemas, `active co "x"`, sqlEvaluator{})
		require.ErrorIs(t, err, ErrInvalidFilter(""))
	})

	t.Run("resolves presence of an unknown attribute to invalidFilter", func(t *testing.T) {
		_, err := Filter[clause](schemas, `nickName pr`, sqlEvaluator{})
		require.ErrorIs(t, err, ErrInvalidFilter(""))
	})

	t.Run("rejects a value path on a non-multivalued attribute", func(t *testing.T) {
		_, err := Filter[clause](schemas, `userName[type eq "work"]`, sqlEvaluator{})
		require.ErrorIs(t, err, ErrInvalidFilter(""))
	})

	t.Run("rejects a value path targeting an unknown attribute", func(t *testing.T) {
		_, err := Filter[clause](schemas, `unknown[type eq "work"]`, sqlEvaluator{})
		require.ErrorIs(t, err, ErrInvalidFilter(""))
	})

	t.Run("rejects an unknown sub-attribute within a value path", func(t *testing.T) {
		_, err := Filter[clause](schemas, `emails[unknownSub eq "work"]`, sqlEvaluator{})
		require.ErrorIs(t, err, ErrInvalidFilter(""))
	})

	t.Run("rejects an unknown sub-attribute of a known attribute", func(t *testing.T) {
		_, err := Filter[clause](schemas, `emails.unknownSub eq "work"`, sqlEvaluator{})
		require.ErrorIs(t, err, ErrInvalidFilter(""))
	})

	t.Run("rejects any attribute when no schemas are configured", func(t *testing.T) {
		_, err := Filter[clause]([]*core.Schema{}, `userName eq "bob"`, sqlEvaluator{})
		require.ErrorIs(t, err, ErrInvalidFilter(""))
	})

	t.Run("maps an unknown attribute to invalidFilter", func(t *testing.T) {
		_, err := Filter[clause](schemas, `nickName eq "x"`, sqlEvaluator{})
		require.ErrorIs(t, err, ErrInvalidFilter(""))
	})

	t.Run("maps an unknown schema URI to invalidFilter", func(t *testing.T) {
		_, err := Filter[clause](schemas, `urn:example:Widget:color eq "red"`, sqlEvaluator{})
		require.ErrorIs(t, err, ErrInvalidFilter(""))
	})

	t.Run("maps a mistyped value to invalidValue", func(t *testing.T) {
		_, err := Filter[clause](schemas, `active eq "yes"`, sqlEvaluator{})
		require.ErrorIs(t, err, ErrInvalidValue(""))
	})

	t.Run("rejects an operator that is invalid for the attribute type", func(t *testing.T) {
		_, err := Filter[clause](schemas, `active gt true`, sqlEvaluator{})
		require.ErrorIs(t, err, ErrInvalidFilter(""))
	})

	t.Run("maps a malformed filter to invalidFilter", func(t *testing.T) {
		_, err := Filter[clause](schemas, `userName zz "x"`, sqlEvaluator{})
		require.ErrorIs(t, err, ErrInvalidFilter(""))
	})
}

var sqlOperators = map[filter.Operator]string{
	filter.OpEquals:            "=",
	filter.OpNotEquals:         "<>",
	filter.OpContains:          "co",
	filter.OpStartsWith:        "sw",
	filter.OpEndsWith:          "ew",
	filter.OpGreaterThan:       ">",
	filter.OpGreaterThanEquals: ">=",
	filter.OpLessThan:          "<",
	filter.OpLessThanEquals:    "<=",
}

type clause struct {
	sql  string
	args []any
}

type sqlEvaluator struct{}

func (sqlEvaluator) Compare(_ *core.Attribute, key string, op filter.Operator, value any) (clause, error) {
	symbol, ok := sqlOperators[op]
	if !ok {
		return clause{}, ErrInvalidFilter(fmt.Sprintf("operator %q is not supported", op))
	}
	return clause{sql: key + " " + symbol + " ?", args: []any{value}}, nil
}

func (sqlEvaluator) Present(_ *core.Attribute, key string) (clause, error) {
	return clause{sql: key + " IS NOT NULL"}, nil
}

func (sqlEvaluator) And(left, right clause) (clause, error) {
	return clause{sql: "(" + left.sql + " AND " + right.sql + ")", args: concat(left.args, right.args)}, nil
}

func (sqlEvaluator) Or(left, right clause) (clause, error) {
	return clause{sql: "(" + left.sql + " OR " + right.sql + ")", args: concat(left.args, right.args)}, nil
}

func (sqlEvaluator) Not(operand clause) (clause, error) {
	return clause{sql: "NOT (" + operand.sql + ")", args: operand.args}, nil
}

func (sqlEvaluator) ValuePath(_ *core.Attribute, key string, valueFilter func() (clause, error)) (clause, error) {
	inner, err := valueFilter()
	if err != nil {
		return clause{}, err
	}
	return clause{sql: "EXISTS(" + key + ": " + inner.sql + ")", args: inner.args}, nil
}

func concat(a, b []any) []any {
	return append(append([]any{}, a...), b...)
}
