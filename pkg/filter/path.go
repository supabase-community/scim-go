package filter

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
	sub := node.SubAttribute()
	if sub == "" {
		return path, nil
	}
	if path.SubAttribute != "" {
		return Path{}, newParseError(text, len(text)-len(sub)-1)
	}
	path.SubAttribute = sub
	return path, nil
}
