package filter_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/filter/internal/peg"
)

func TestNode(t *testing.T) {
	t.Run("leaf comparison", func(t *testing.T) {
		n := filter.NewNode(peg.Token{"attribute": "userName", "operator": "eq", "value": "bjensen"})

		assert.Equal(t, "userName", n.Attribute())
		assert.Equal(t, "eq", n.Operator())
		assert.Equal(t, "bjensen", n.Value())
		assert.False(t, n.Not())
	})

	t.Run("presence", func(t *testing.T) {
		n := filter.NewNode(peg.Token{"attribute": "userName", "operator": "pr"})

		assert.Equal(t, "userName", n.Attribute())
		assert.Equal(t, "pr", n.Operator())
		assert.Nil(t, n.Value())
	})

	t.Run("logical and/or", func(t *testing.T) {
		left := peg.Token{"attribute": "a", "operator": "eq", "value": "1"}
		right := peg.Token{"attribute": "b", "operator": "eq", "value": "2"}
		n := filter.NewNode(peg.Token{"left": left, "operator": "and", "right": right})

		assert.Equal(t, "and", n.Operator())
		assert.Equal(t, "a", n.Left().Attribute())
		assert.Equal(t, "b", n.Right().Attribute())
	})

	t.Run("not wraps a nested operand rather than flagging the same node", func(t *testing.T) {
		n := filter.NewNode(peg.Token{"not": peg.Token{"attribute": "userName", "operator": "pr"}})

		assert.True(t, n.Not())
		assert.Equal(t, "userName", n.Operand().Attribute())
		assert.Equal(t, "pr", n.Operand().Operator())
	})

	t.Run("nested not stays distinct at each level", func(t *testing.T) {
		innermost := peg.Token{"attribute": "userName", "operator": "pr"}
		n := filter.NewNode(peg.Token{"not": peg.Token{"not": innermost}})

		assert.True(t, n.Not())
		assert.True(t, n.Operand().Not())
		assert.Equal(t, "userName", n.Operand().Operand().Attribute())
	})

	t.Run("valuePath", func(t *testing.T) {
		valueFilter := peg.Token{"attribute": "type", "operator": "eq", "value": "work"}
		n := filter.NewNode(peg.Token{"path": "emails", "value_filter": valueFilter, "sub_attribute": "value"})

		assert.True(t, n.HasPath())
		assert.Equal(t, "emails", n.Path())
		assert.Equal(t, "value", n.SubAttribute())
		assert.Equal(t, "type", n.ValueFilter().Attribute())
	})

	t.Run("valuePath without a sub-attribute", func(t *testing.T) {
		valueFilter := peg.Token{"attribute": "type", "operator": "eq", "value": "work"}
		n := filter.NewNode(peg.Token{"path": "emails", "value_filter": valueFilter})

		assert.Empty(t, n.SubAttribute())
	})

	t.Run("NewNode returns nil for a non-Token value", func(t *testing.T) {
		assert.Nil(t, filter.NewNode("not a token"))
		assert.Nil(t, filter.NewNode(nil))
	})
}

func TestNodeAttrPathSplitsParts(t *testing.T) {
	tt := []struct {
		input string
		want  filter.AttrPath
	}{
		{`userName eq "x"`, filter.AttrPath{Name: "userName"}},
		{`name.familyName eq "x"`, filter.AttrPath{Name: "name", SubAttribute: "familyName"}},
		{
			`urn:ietf:params:scim:schemas:core:2.0:User:userName eq "x"`,
			filter.AttrPath{
				URI:  "urn:ietf:params:scim:schemas:core:2.0:User",
				Name: "userName",
			},
		},
		{
			`urn:ietf:params:scim:schemas:core:2.0:User:name.familyName eq "x"`,
			filter.AttrPath{
				URI:          "urn:ietf:params:scim:schemas:core:2.0:User",
				Name:         "name",
				SubAttribute: "familyName",
			},
		},
	}
	g := filter.New(0)
	for _, tc := range tt {
		t.Run(tc.input, func(t *testing.T) {
			node, err := g.Parse(tc.input)
			require.NoError(t, err)

			got := node.AttrPath()

			assert.Equal(t, tc.want, got)
			assert.Equal(t, node.Attribute(), got.String())
		})
	}
}

func TestNodeAttrPathOfValuePath(t *testing.T) {
	g := filter.New(0)
	node, err := g.Parse(`emails[type eq "work"]`)
	require.NoError(t, err)

	assert.Equal(t, filter.AttrPath{Name: "emails"}, node.AttrPath())
}
