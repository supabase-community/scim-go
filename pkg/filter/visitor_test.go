package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringifyVisitorRoundTrip(t *testing.T) {
	tt := []string{
		`userName eq "bjensen"`,
		`userName pr`,
		`age gt 21`,
		`active eq true`,
		`nickName eq null`,
		`userName eq "bjensen" and active eq true`,
		`a eq "1" and b eq "2" or c eq "3"`,
		`not (userName eq "bjensen")`,
		`not (not (userName pr))`,
		`(a eq "1" or b eq "2") and c eq "3"`,
		`emails[type eq "work"]`,
		`emails[type eq "work"].value`,
		`emails[type eq "work" and primary eq true]`,
	}

	g := &Grammar{}
	for _, input := range tt {
		t.Run(input, func(t *testing.T) {
			original, err := g.Parse(input)
			require.NoError(t, err)

			text, err := Visit[string](StringifyVisitor{}, original)
			require.NoError(t, err)

			reparsed, err := g.Parse(text)
			require.NoError(t, err)

			assert.Equal(t, original, reparsed)
		})
	}
}

type countingVisitor struct{}

func (countingVisitor) VisitAnd(left, right int) (int, error)             { return left + right, nil }
func (countingVisitor) VisitOr(left, right int) (int, error)              { return left + right, nil }
func (countingVisitor) VisitNot(operand int) (int, error)                 { return operand + 100, nil }
func (countingVisitor) VisitEquals(AttrPath, any) (int, error)            { return 1, nil }
func (countingVisitor) VisitNotEquals(AttrPath, any) (int, error)         { return 1, nil }
func (countingVisitor) VisitContains(AttrPath, any) (int, error)          { return 1, nil }
func (countingVisitor) VisitStartsWith(AttrPath, any) (int, error)        { return 1, nil }
func (countingVisitor) VisitEndsWith(AttrPath, any) (int, error)          { return 1, nil }
func (countingVisitor) VisitGreaterThan(AttrPath, any) (int, error)       { return 1, nil }
func (countingVisitor) VisitGreaterThanEquals(AttrPath, any) (int, error) { return 1, nil }
func (countingVisitor) VisitLessThan(AttrPath, any) (int, error)          { return 1, nil }
func (countingVisitor) VisitLessThanEquals(AttrPath, any) (int, error)    { return 1, nil }
func (countingVisitor) VisitPresence(AttrPath) (int, error)               { return 1, nil }
func (countingVisitor) VisitValuePath(_ AttrPath, _ string, valueFilter func() (int, error)) (int, error) {
	return valueFilter()
}

func TestVisitDispatch(t *testing.T) {
	g := &Grammar{}

	t.Run("counts leaves across and/or", func(t *testing.T) {
		node, err := g.Parse(`a eq "1" and b eq "2" and c eq "3"`)
		require.NoError(t, err)

		count, err := Visit[int](countingVisitor{}, node)

		require.NoError(t, err)
		assert.Equal(t, 3, count)
	})

	t.Run("not adds 100 on top of the underlying result", func(t *testing.T) {
		node, err := g.Parse(`not (userName eq "bjensen")`)
		require.NoError(t, err)

		count, err := Visit[int](countingVisitor{}, node)

		require.NoError(t, err)
		assert.Equal(t, 101, count)
	})

	t.Run("nil node is an error, not a panic", func(t *testing.T) {
		_, err := Visit[int](countingVisitor{}, nil)

		assert.Error(t, err)
	})
}

type scopeVisitor struct{ scope []string }

func (v *scopeVisitor) qualify(attribute AttrPath) string {
	if len(v.scope) == 0 {
		return attribute.String()
	}
	return v.scope[len(v.scope)-1] + "." + attribute.String()
}

func (v *scopeVisitor) VisitAnd(l, r string) (string, error) { return l + " AND " + r, nil }
func (v *scopeVisitor) VisitOr(l, r string) (string, error)  { return l + " OR " + r, nil }
func (v *scopeVisitor) VisitNot(o string) (string, error)    { return "NOT " + o, nil }
func (v *scopeVisitor) VisitEquals(a AttrPath, _ any) (string, error) {
	return v.qualify(a) + " = ?", nil
}
func (v *scopeVisitor) VisitNotEquals(a AttrPath, _ any) (string, error)   { return v.qualify(a), nil }
func (v *scopeVisitor) VisitContains(a AttrPath, _ any) (string, error)    { return v.qualify(a), nil }
func (v *scopeVisitor) VisitStartsWith(a AttrPath, _ any) (string, error)  { return v.qualify(a), nil }
func (v *scopeVisitor) VisitEndsWith(a AttrPath, _ any) (string, error)    { return v.qualify(a), nil }
func (v *scopeVisitor) VisitGreaterThan(a AttrPath, _ any) (string, error) { return v.qualify(a), nil }
func (v *scopeVisitor) VisitGreaterThanEquals(a AttrPath, _ any) (string, error) {
	return v.qualify(a), nil
}
func (v *scopeVisitor) VisitLessThan(a AttrPath, _ any) (string, error) { return v.qualify(a), nil }
func (v *scopeVisitor) VisitLessThanEquals(a AttrPath, _ any) (string, error) {
	return v.qualify(a), nil
}
func (v *scopeVisitor) VisitPresence(a AttrPath) (string, error) { return v.qualify(a), nil }
func (v *scopeVisitor) VisitValuePath(path AttrPath, _ string, valueFilter func() (string, error)) (string, error) {
	v.scope = append(v.scope, path.String())
	inner, err := valueFilter()
	v.scope = v.scope[:len(v.scope)-1]
	if err != nil {
		return "", err
	}
	return "EXISTS(" + inner + ")", nil
}

func TestVisitValuePathThreadsScope(t *testing.T) {
	g := &Grammar{}
	node, err := g.Parse(`emails[type eq "work" and primary eq true]`)
	require.NoError(t, err)

	got, err := Visit[string](&scopeVisitor{}, node)

	require.NoError(t, err)
	assert.Equal(t, "EXISTS(emails.type = ? AND emails.primary = ?)", got)
}
