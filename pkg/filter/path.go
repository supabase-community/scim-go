package filter

import "strings"

// Path is a parsed PATCH path: attrPath / valuePath [subAttr] (RFC 7644 3.5.2).
type Path struct {
	AttrPath
	ValueFilter *Node
}

func NewPath(text string) (Path, error) {
	if strings.IndexByte(text, '[') < 0 {
		path, err := NewAttrPath(text)
		if err != nil {
			return Path{}, err
		}
		return Path{AttrPath: path}, nil
	}
	// RFC 7644 Section 3.5.2: PATH = attrPath / valuePath [subAttr], not FILTER.
	raw, err := defaultGrammar.run(text, defaultGrammar.valuePath(defaultGrammar.valueFilter))
	if err != nil {
		return Path{}, err
	}
	node := NewNode(raw)
	path := Path{
		AttrPath:    splitAttrPath(node.Path()),
		ValueFilter: node.ValueFilter(),
	}
	if sub := node.SubAttribute(); sub != "" {
		path.SubAttribute = sub
	}
	return path, nil
}
