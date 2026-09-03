package peg

func Ref(p *Parser) Parser {
	return func(c *Context) (ASTNode, error) {
		return (*p)(c)
	}
}
