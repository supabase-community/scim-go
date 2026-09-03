package peg

import (
	"regexp"
)

func Match(re *regexp.Regexp) Parser {
	return func(c *Context) (ASTNode, error) {
		loc := re.FindStringIndex(c.stream[c.position:])
		if loc == nil || loc[0] != 0 {
			return nil, ErrNoMatch
		}
		res := c.stream[c.position : c.position+loc[1]]
		c.position += loc[1]
		return res, nil
	}
}
