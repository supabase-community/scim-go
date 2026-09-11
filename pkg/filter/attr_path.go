package filter

import "strings"

// AttrPath is a parsed attrPath = [URI ":"] ATTRNAME *1subAttr (RFC 7644 3.4.2.2).
type AttrPath struct {
	URI          string
	Name         string
	SubAttribute string
}

func NewAttrPath(text string) (AttrPath, error) {
	raw, err := DefaultGrammar.run(text, DefaultGrammar.attributePath())
	if err != nil {
		return AttrPath{}, err
	}
	return newAttrPath(raw.(string)), nil
}

func newAttrPath(raw string) AttrPath {
	var path AttrPath
	if i := strings.LastIndex(raw, ":"); i >= 0 {
		path.URI, raw = raw[:i], raw[i+1:]
	}
	if i := strings.IndexByte(raw, '.'); i >= 0 {
		path.Name, path.SubAttribute = raw[:i], raw[i+1:]
	} else {
		path.Name = raw
	}
	return path
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
