package filter

import "github.com/supabase-community/scim-go/pkg/filter/internal/peg"

type Node struct {
	raw peg.Token
}

func NewNode(raw peg.ASTNode) *Node {
	token, ok := raw.(peg.Token)
	if !ok {
		return nil
	}
	return &Node{raw: token}
}

func (n *Node) Operator() string {
	s, _ := n.raw["operator"].(string)
	return s
}

func (n *Node) Attribute() string {
	s, _ := n.raw["attribute"].(string)
	return s
}

func (n *Node) AttrPath() AttrPath {
	if n.HasPath() {
		return newAttrPath(n.Path())
	}
	return newAttrPath(n.Attribute())
}

func (n *Node) Value() any {
	return n.raw["value"]
}

func (n *Node) Not() bool {
	return n.hasKey("not")
}

func (n *Node) Operand() *Node {
	return NewNode(n.raw["not"])
}

func (n *Node) Left() *Node {
	return NewNode(n.raw["left"])
}

func (n *Node) Right() *Node {
	return NewNode(n.raw["right"])
}

func (n *Node) HasPath() bool {
	return n.hasKey("path")
}

func (n *Node) Path() string {
	s, _ := n.raw["path"].(string)
	return s
}

func (n *Node) ValueFilter() *Node {
	return NewNode(n.raw["value_filter"])
}

func (n *Node) SubAttribute() string {
	s, _ := n.raw["sub_attribute"].(string)
	return s
}

func (n *Node) hasKey(key string) bool {
	_, ok := n.raw[key]
	return ok
}
