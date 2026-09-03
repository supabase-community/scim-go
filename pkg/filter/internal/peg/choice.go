package peg

func Choice(parslets ...Parser) Parser {
	return func(c *Context) (ASTNode, error) {
		start := c.position
		for _, p := range parslets {
			if val, err := p(c); err == nil {
				return val, nil
			}
			c.position = start
		}
		return nil, ErrNoMatch
	}
}
