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

	t.Run("renders the resolved key, operator, and coerced value", func(t *testing.T) {
		out, err := Filter[string](schemas, `active eq true`, sqlEvaluator{})
		require.NoError(t, err)
		assert.Equal(t, "active = true", out)
	})

	t.Run("preserves the dotted key for a common sub-attribute", func(t *testing.T) {
		out, err := Filter[string](schemas, `meta.lastModified gt "2020-01-01T00:00:00Z"`, sqlEvaluator{})
		require.NoError(t, err)
		assert.Contains(t, out, "meta.lastmodified > ")
	})

	t.Run("composes and, or, not", func(t *testing.T) {
		out, err := Filter[string](schemas, `userName eq "bob" and not (active eq false)`, sqlEvaluator{})
		require.NoError(t, err)
		assert.Equal(t, "(username = bob AND NOT (active = false))", out)
	})

	t.Run("resolves presence", func(t *testing.T) {
		out, err := Filter[string](schemas, `userName pr`, sqlEvaluator{})
		require.NoError(t, err)
		assert.Equal(t, "username IS NOT NULL", out)
	})

	t.Run("scopes a value path to the element sub-attributes", func(t *testing.T) {
		out, err := Filter[string](schemas, `emails[type eq "work"]`, sqlEvaluator{})
		require.NoError(t, err)
		assert.Equal(t, "EXISTS(emails: type = work)", out)
	})

	t.Run("composes a value path with a sibling comparison", func(t *testing.T) {
		out, err := Filter[string](schemas, `emails[type eq "work"] and userName eq "bob"`, sqlEvaluator{})
		require.NoError(t, err)
		assert.Equal(t, "(EXISTS(emails: type = work) AND username = bob)", out)
	})

	t.Run("resolves an extension attribute by schema URI", func(t *testing.T) {
		enterprise := (&core.Schema{ID: core.SchemaEnterpriseUser, Name: "EnterpriseUser"}).With(
			core.NewAttribute("department", core.TypeString, ""),
		)
		text := fmt.Sprintf("%s:department eq \"eng\"", core.SchemaEnterpriseUser)
		out, err := Filter[string]([]*core.Schema{schema, enterprise}, text, sqlEvaluator{})
		require.NoError(t, err)
		assert.Equal(t, "department = eng", out)
	})

	t.Run("renders gt, ge, lt, le comparisons", func(t *testing.T) {
		cases := map[string]string{
			`userName gt "a"`: "username > a",
			`userName ge "a"`: "username >= a",
			`userName lt "z"`: "username < z",
			`userName le "z"`: "username <= z",
		}
		for text, want := range cases {
			out, err := Filter[string](schemas, text, sqlEvaluator{})
			require.NoError(t, err)
			assert.Equal(t, want, out)
		}
	})

	t.Run("composes or", func(t *testing.T) {
		out, err := Filter[string](schemas, `userName eq "bob" or active eq true`, sqlEvaluator{})
		require.NoError(t, err)
		assert.Equal(t, "(username = bob OR active = true)", out)
	})

	t.Run("renders an integer comparison", func(t *testing.T) {
		out, err := Filter[string](schemas, `age gt 21`, sqlEvaluator{})
		require.NoError(t, err)
		assert.Equal(t, "age > 21", out)
	})

	t.Run("preserves an integer beyond float64 precision", func(t *testing.T) {
		out, err := Filter[string](schemas, `age eq 9007199254740993`, sqlEvaluator{})
		require.NoError(t, err)
		assert.Equal(t, "age = 9007199254740993", out)
	})

	t.Run("maps an unknown attribute to invalidFilter", func(t *testing.T) {
		_, err := Filter[string](schemas, `nickName eq "x"`, sqlEvaluator{})
		require.ErrorIs(t, err, ErrInvalidFilter(""))
	})

	t.Run("maps an unknown schema URI to invalidFilter", func(t *testing.T) {
		_, err := Filter[string](schemas, `urn:example:Widget:color eq "red"`, sqlEvaluator{})
		require.ErrorIs(t, err, ErrInvalidFilter(""))
	})

	t.Run("maps a mistyped value to invalidValue", func(t *testing.T) {
		_, err := Filter[string](schemas, `active eq "yes"`, sqlEvaluator{})
		require.ErrorIs(t, err, ErrInvalidValue(""))
	})

	t.Run("rejects an operator that is invalid for the attribute type", func(t *testing.T) {
		_, err := Filter[string](schemas, `active gt true`, sqlEvaluator{})
		require.ErrorIs(t, err, ErrInvalidFilter(""))
	})

	t.Run("maps a malformed filter to invalidFilter", func(t *testing.T) {
		_, err := Filter[string](schemas, `userName zz "x"`, sqlEvaluator{})
		require.ErrorIs(t, err, ErrInvalidFilter(""))
	})
}

var sqlOperators = map[filter.Operator]string{
	filter.OpEquals:            "=",
	filter.OpNotEquals:         "<>",
	filter.OpGreaterThan:       ">",
	filter.OpGreaterThanEquals: ">=",
	filter.OpLessThan:          "<",
	filter.OpLessThanEquals:    "<=",
}

type sqlEvaluator struct{}

func (sqlEvaluator) Compare(_ *core.Attribute, key string, op filter.Operator, value any) (string, error) {
	symbol, ok := sqlOperators[op]
	if !ok {
		return "", ErrInvalidFilter(fmt.Sprintf("operator %q is not supported", op))
	}
	return fmt.Sprintf("%s %s %v", key, symbol, value), nil
}

func (sqlEvaluator) Present(_ *core.Attribute, key string) (string, error) {
	return key + " IS NOT NULL", nil
}

func (sqlEvaluator) And(left, right string) (string, error) {
	return "(" + left + " AND " + right + ")", nil
}

func (sqlEvaluator) Or(left, right string) (string, error) {
	return "(" + left + " OR " + right + ")", nil
}

func (sqlEvaluator) Not(operand string) (string, error) {
	return "NOT (" + operand + ")", nil
}

func (sqlEvaluator) ValuePath(_ *core.Attribute, key string, valueFilter func() (string, error)) (string, error) {
	inner, err := valueFilter()
	if err != nil {
		return "", err
	}
	return "EXISTS(" + key + ": " + inner + ")", nil
}
