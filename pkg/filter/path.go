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
		path, err := ParseAttrPath(text)
		if err != nil {
			return Path{}, err
		}
		return Path{AttrPath: path}, nil
	}
	node, err := Parse(text)
	if err != nil {
		return Path{}, err
	}
	if !node.HasPath() {
		return Path{}, &ParseError{Input: text, Position: 0}
	}
	path := Path{
		AttrPath:    newAttrPath(node.Path()),
		ValueFilter: node.ValueFilter(),
	}
	if sub := node.SubAttribute(); sub != "" {
		path.SubAttribute = sub
	}
	return path, nil
}
