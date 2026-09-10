package filter

import "strings"

// AttrPath is a parsed attrPath = [URI ":"] ATTRNAME *1subAttr (RFC 7644 3.4.2.2).
type AttrPath struct {
	URI          string
	Name         string
	SubAttribute string
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

func parseAttrPath(s string) AttrPath {
	var p AttrPath
	if i := strings.LastIndex(s, ":"); i >= 0 {
		p.URI, s = s[:i], s[i+1:]
	}
	if i := strings.IndexByte(s, '.'); i >= 0 {
		p.Name, p.SubAttribute = s[:i], s[i+1:]
	} else {
		p.Name = s
	}
	return p
}
