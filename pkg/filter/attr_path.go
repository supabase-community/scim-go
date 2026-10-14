package filter

import "strings"

// AttrPath is a parsed attrPath = [URI ":"] ATTRNAME *1subAttr, per RFC 7644 Section 3.4.2.2.
type AttrPath struct {
	URI          string
	Name         string
	SubAttribute string
}

func NewAttrPath(text string) (AttrPath, error) {
	raw, err := defaultGrammar.run(text, defaultGrammar.attrPath)
	if err != nil {
		return AttrPath{}, err
	}
	return splitAttrPath(asString(raw)), nil
}

func (p AttrPath) String() string {
	s := p.Name
	if p.SubAttribute != "" {
		s += "." + p.SubAttribute
	}
	if p.URI != "" {
		s = p.URI + ":" + s
	}
	return s
}

func (p AttrPath) Key() string {
	if p.SubAttribute == "" {
		return strings.ToLower(p.Name)
	}
	return strings.ToLower(p.Name + "." + p.SubAttribute)
}

func splitAttrPath(raw string) AttrPath {
	var path AttrPath
	if uri, rest, found := strings.CutLast(raw, ":"); found {
		path.URI, raw = uri, rest
	}
	path.Name, path.SubAttribute, _ = strings.Cut(raw, ".")
	return path
}
