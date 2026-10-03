package filter

import "strings"

// Path is a parsed PATCH path, per RFC 7644 Section 3.5.2.
type Path struct {
	AttrPath
	ValueFilter *Node
}

func NewPath(text string) (Path, error) {
	node, err := defaultGrammar.parse(text, defaultGrammar.path)
	if err != nil {
		return Path{}, err
	}
	path := Path{AttrPath: node.AttrPath(), ValueFilter: node.ValueFilter()}
	if path.ValueFilter == nil {
		return path, nil
	}
	if path.SubAttribute != "" {
		return Path{}, newParseError(text, strings.IndexByte(text, '['))
	}
	path.SubAttribute = node.SubAttribute()
	return path, nil
}
