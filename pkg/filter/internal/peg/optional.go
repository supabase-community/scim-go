package peg

func Optional(p Parser) Parser {
	return func(c *Context) (ASTNode, error) {
		start := c.position
		if val, err := p(c); err == nil {
			return val, nil
		}
		c.position = start
		return nil, nil
	}
}
