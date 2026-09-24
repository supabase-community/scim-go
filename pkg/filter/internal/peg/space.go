package peg

// Space matches 1*SP, per RFC 5234, Appendix B.1.
func Space() Parser {
	return func(c *Context) (ASTNode, error) {
		start := c.position
		for c.position < len(c.stream) && c.stream[c.position] == ' ' {
			c.position++
		}
		if c.position == start {
			return nil, ErrNoMatch
		}
		return nil, nil
	}
}
