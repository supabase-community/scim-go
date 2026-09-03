package peg

import "errors"

type ASTNode any
type Token map[string]ASTNode
type Parser func(c *Context) (ASTNode, error)

var ErrNoMatch = errors.New("peg: no match")
