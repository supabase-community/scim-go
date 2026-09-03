package filter

import (
	"reflect"
	"testing"
)

func FuzzParse(f *testing.F) {
	seeds := []string{
		`userName eq "bjensen"`,
		`userName pr`,
		`age gt 21`,
		`active eq true`,
		`emails[type eq "work"].value`,
		`emails[type eq "work" and primary eq true]`,
		`not (a eq "1" and b pr) or c sw "x"`,
		`urn:ietf:params:scim:schemas:core:2.0:User:userName eq "x"`,
		`(a eq "1" or b eq "2") and c eq "3"`,
		``,
		`(((`,
		`userName EQ "x" AND active PR`,
	}
	for _, s := range seeds {
		f.Add(s)
	}

	g := New()
	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 4096 {
			return
		}
		node, err := g.Parse(input)
		if err != nil {
			return
		}
		if node == nil {
			t.Fatalf("nil node with nil error for %q", input)
		}

		text, err := Visit[string](StringifyVisitor{}, node)
		if err != nil {
			return
		}
		reparsed, err := g.Parse(text)
		if err != nil {
			t.Fatalf("stringified %q -> %q failed to reparse: %v", input, text, err)
		}
		if !reflect.DeepEqual(node, reparsed) {
			t.Fatalf("round-trip changed the AST: %q -> %q", input, text)
		}
	})
}
