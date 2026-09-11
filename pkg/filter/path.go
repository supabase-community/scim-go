package filter

import "strings"

// Path is a parsed PATCH path: attrPath / valuePath [subAttr] (RFC 7644 3.5.2).
type Path struct {
	AttrPath
	ValueFilter *Node
}

// NewPath parses a SCIM PATCH path, per RFC 7644, Section 3.5.2.
func NewPath(text string) (Path, error) {
	if strings.IndexByte(text, '[') < 0 {
		p, err := ParseAttrPath(text)
		if err != nil {
			return Path{}, err
		}
		return Path{AttrPath: p}, nil
	}
	node, err := Parse(text)
	if err != nil {
		return Path{}, err
	}
	if !node.HasPath() {
		return Path{}, &ParseError{Input: text, Position: 0}
	}
	p := Path{AttrPath: parseAttrPath(node.Path()), ValueFilter: node.ValueFilter()}
	if sub := node.SubAttribute(); sub != "" {
		p.SubAttribute = sub
	}
	return p, nil
}
