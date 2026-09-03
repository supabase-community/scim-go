package peg

func Sequence(parslets ...Parser) Parser {
	return func(c *Context) (ASTNode, error) {
		start := c.position
		var results []ASTNode
		for _, p := range parslets {
			val, err := p(c)
			if err != nil {
				c.position = start
				return nil, err
			}
			if val != nil {
				results = append(results, val)
			}
		}
		var merged Token
		for _, r := range results {
			token, ok := r.(Token)
			if !ok {
				continue
			}
			if merged == nil {
				merged = Token{}
			}
			for k, v := range token {
				merged[k] = v
			}
		}
		if merged == nil {
			return results, nil
		}
		return merged, nil
	}
}
