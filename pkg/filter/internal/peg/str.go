package peg

// Str matches a case-sensitive literal.
func Str(s string) Parser {
	return func(c *Context) (ASTNode, error) {
		if c.position+len(s) <= len(c.stream) && c.stream[c.position:c.position+len(s)] == s {
			c.position += len(s)
			return s, nil
		}
		return nil, ErrNoMatch
	}
}
