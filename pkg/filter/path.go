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
	if sub := node.SubAttribute(); sub != "" {
		path.SubAttribute = sub
	}
	return path, nil
}
