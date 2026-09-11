package filter

import "strings"

// Path is a parsed PATCH path: attrPath / valuePath [subAttr] (RFC 7644 3.5.2).
type Path struct {
	AttrPath
	ValueFilter *Node
}

// ParsePath parses a SCIM PATCH path, per RFC 7644, Section 3.5.2. A bare
// attrPath (userName, name.familyName) has a nil ValueFilter; a valuePath
// (emails[type eq "work"].value) carries the bracketed filter as ValueFilter.
func ParsePath(text string) (Path, error) {
	if strings.IndexByte(text, '[') < 0 {
		p := parseAttrPath(text)
		if !validName(p.Name) || (p.SubAttribute != "" && !validName(p.SubAttribute)) || p.String() != text {
			return Path{}, &ParseError{Input: text, Position: 0}
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

func validName(s string) bool {
	return s != "" && reAttributeName.FindString(s) == s
}
