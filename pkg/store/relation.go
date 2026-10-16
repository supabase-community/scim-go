package store

import (
	"bytes"
	"cmp"
	"encoding/json"
	"encoding/json/jsontext"
	"slices"

	"github.com/supabase-community/scim-go/pkg/core"
)

type Relation struct {
	Reference Reference
	ID        string
	Type      string
}

func Nest(object core.Object, relations []Relation) core.Object {
	for _, relation := range relations {
		holder := relation.Reference.holder(object, true)
		members, _ := holder.Get(relation.Reference.Name).([]any)
		holder.Set(relation.Reference.Name, append(members, relation.member()))
	}
	return object
}

// RFC 7643 Section 4.1.2: groups.type is direct or indirect. Section 4.2: members.type is the resource type. Type is empty for a direct group.
func (r Relation) member() core.Object {
	return core.Object{ValueAttribute: r.ID, "type": r.kind()}
}

func (r Relation) kind() string {
	if r.Type == "" && r.Reference.Inverse() {
		return "direct"
	}
	return r.Type
}

func (r Relation) extensionID() string {
	if r.Reference.extension == nil {
		return ""
	}
	return string(r.Reference.extension.ID)
}

func (r Relation) attribute() string {
	return r.Reference.Name
}

func Embed(body json.RawMessage, relations []Relation) json.RawMessage {
	if len(relations) == 0 {
		return body
	}
	head := bytes.TrimRight(body, " \t\r\n")
	head = head[:len(head)-1]
	nested := appendRelations(nil, relations)
	out := append(make([]byte, 0, len(head)+len(nested)), head...)
	if len(bytes.TrimSpace(head)) > 1 {
		out = append(out, ',')
	}
	return append(out, nested[1:]...)
}

func appendRelations(dst []byte, relations []Relation) []byte {
	sorted := slices.SortedStableFunc(slices.Values(relations), compareRelations)
	dst = append(dst, '{')
	for i, run := range runs(sorted, Relation.extensionID) {
		dst = appendExtension(appendComma(dst, i), run)
	}
	return append(dst, '}')
}

func appendExtension(dst []byte, run []Relation) []byte {
	id := run[0].extensionID()
	if id != "" {
		dst = append(appendKey(dst, id), '{')
	}
	for i, attributes := range runs(run, Relation.attribute) {
		dst = appendAttribute(appendComma(dst, i), attributes)
	}
	if id != "" {
		dst = append(dst, '}')
	}
	return dst
}

func appendAttribute(dst []byte, run []Relation) []byte {
	dst = append(appendKey(dst, run[0].attribute()), '[')
	for i, relation := range run {
		dst = relation.appendMember(appendComma(dst, i))
	}
	return append(dst, ']')
}

func (r Relation) appendMember(dst []byte) []byte {
	dst = appendQuote(appendKey(append(dst, '{'), ValueAttribute), r.ID)
	dst = appendQuote(appendKey(append(dst, ','), "type"), r.kind())
	return append(dst, '}')
}

func appendKey(dst []byte, name string) []byte {
	return append(appendQuote(dst, name), ':')
}

func appendQuote(dst []byte, s string) []byte {
	dst, _ = jsontext.AppendQuote(dst, s)
	return dst
}

func appendComma(dst []byte, index int) []byte {
	if index > 0 {
		return append(dst, ',')
	}
	return dst
}

func compareRelations(a, b Relation) int {
	return cmp.Or(cmp.Compare(a.extensionID(), b.extensionID()), cmp.Compare(a.attribute(), b.attribute()))
}

func runs(sorted []Relation, key func(Relation) string) [][]Relation {
	var out [][]Relation
	for start := 0; start < len(sorted); {
		end := start + 1
		for end < len(sorted) && key(sorted[end]) == key(sorted[start]) {
			end++
		}
		out = append(out, sorted[start:end])
		start = end
	}
	return out
}
