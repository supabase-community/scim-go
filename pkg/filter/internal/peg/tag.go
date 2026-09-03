package peg

func Tag(key string, p Parser) Parser {
	return func(c *Context) (ASTNode, error) {
		val, err := p(c)
		if err != nil {
			return nil, err
		}
		return Token{key: val}, nil
	}
}
