package peg

import "errors"

type (
	ASTNode any
	Token   map[string]ASTNode
	Parser  func(c *Context) (ASTNode, error)
)

var ErrNoMatch = errors.New("peg: no match")
