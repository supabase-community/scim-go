package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/supabase-community/scim-go/pkg/filter/internal/peg"
)

func TestNode(t *testing.T) {
	t.Run("leaf comparison", func(t *testing.T) {
		n := newNode(peg.Token{"attribute": "userName", "operator": "eq", "value": "bjensen"})

		assert.Equal(t, "userName", n.Attribute())
		assert.Equal(t, "eq", n.Operator())
		assert.Equal(t, "bjensen", n.Value())
		assert.False(t, n.Not())
	})

	t.Run("presence", func(t *testing.T) {
		n := newNode(peg.Token{"attribute": "userName", "operator": "pr"})

		assert.Equal(t, "userName", n.Attribute())
		assert.Equal(t, "pr", n.Operator())
		assert.Nil(t, n.Value())
	})

	t.Run("logical and/or", func(t *testing.T) {
		left := peg.Token{"attribute": "a", "operator": "eq", "value": "1"}
		right := peg.Token{"attribute": "b", "operator": "eq", "value": "2"}
		n := newNode(peg.Token{"left": left, "operator": "and", "right": right})

		assert.Equal(t, "and", n.Operator())
		assert.Equal(t, "a", n.Left().Attribute())
		assert.Equal(t, "b", n.Right().Attribute())
	})

	t.Run("not wraps a nested operand rather than flagging the same node", func(t *testing.T) {
		n := newNode(peg.Token{"not": peg.Token{"attribute": "userName", "operator": "pr"}})

		assert.True(t, n.Not())
		assert.Equal(t, "userName", n.Operand().Attribute())
		assert.Equal(t, "pr", n.Operand().Operator())
	})

	t.Run("nested not stays distinct at each level", func(t *testing.T) {
		innermost := peg.Token{"attribute": "userName", "operator": "pr"}
		n := newNode(peg.Token{"not": peg.Token{"not": innermost}})

		assert.True(t, n.Not())
		assert.True(t, n.Operand().Not())
		assert.Equal(t, "userName", n.Operand().Operand().Attribute())
	})

	t.Run("valuePath", func(t *testing.T) {
		valueFilter := peg.Token{"attribute": "type", "operator": "eq", "value": "work"}
		n := newNode(peg.Token{"path": "emails", "value_filter": valueFilter, "sub_attribute": "value"})

		assert.True(t, n.HasPath())
		assert.Equal(t, "emails", n.Path())
		assert.Equal(t, "value", n.SubAttribute())
		assert.Equal(t, "type", n.ValueFilter().Attribute())
	})

	t.Run("valuePath without a sub-attribute", func(t *testing.T) {
		valueFilter := peg.Token{"attribute": "type", "operator": "eq", "value": "work"}
		n := newNode(peg.Token{"path": "emails", "value_filter": valueFilter})

		assert.Equal(t, "", n.SubAttribute())
	})

	t.Run("newNode returns nil for a non-Token value", func(t *testing.T) {
		assert.Nil(t, newNode("not a token"))
		assert.Nil(t, newNode(nil))
	})
}
