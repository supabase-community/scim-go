package peg

import "strings"

// Fold matches a literal case-insensitively
func Fold(s string) Parser {
	return func(c *Context) (ASTNode, error) {
		if c.position+len(s) <= len(c.stream) && strings.EqualFold(c.stream[c.position:c.position+len(s)], s) {
			c.position += len(s)
			return s, nil
		}
		return nil, ErrNoMatch
	}
}
