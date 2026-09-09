package peg

import "maps"

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
		if merged := mergeTokens(results); merged != nil {
			return merged, nil
		}
		return results, nil
	}
}

func mergeTokens(results []ASTNode) Token {
	var merged Token
	for _, r := range results {
		token, ok := r.(Token)
		if !ok {
			continue
		}
		if merged == nil {
			merged = Token{}
		}
		maps.Copy(merged, token)
	}
	return merged
}
