package filter_test

import (
	"reflect"
	"testing"

	"github.com/supabase-community/scim-go/pkg/filter"
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
		`name..familyName`,
	}
	for _, s := range seeds {
		f.Add(s)
	}

	g := filter.New(0)
	f.Fuzz(func(t *testing.T, input string) {
		checkRoundTrip(t, g, input)
	})
}

func FuzzNewPath(f *testing.F) {
	seeds := []string{
		`userName`,
		`name.familyName`,
		`urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department`,
		`emails[type eq "work"]`,
		`emails[type eq "work"].value`,
		`members[value eq "2819c223" or display sw "B"]`,
		``,
		`emails[`,
		`name.familyName.extra`,
		`emails.value[type eq "work"].display`,
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, input string) {
		if len(input) > 1024 {
			return
		}
		path, err := filter.NewPath(input)
		if err != nil {
			return
		}

		text := pathText(t, path)
		reparsed, err := filter.NewPath(text)
		if err != nil {
			t.Fatalf("path %q -> %q failed to reparse: %v", input, text, err)
		}
		if !reflect.DeepEqual(path, reparsed) {
			t.Fatalf("round-trip changed the path: %q -> %q", input, text)
		}
	})
}

func pathText(t *testing.T, path filter.Path) string {
	t.Helper()
	if path.ValueFilter == nil {
		return path.String()
	}
	valueFilter, err := filter.Visit[string](filter.Stringify{}, path.ValueFilter)
	if err != nil {
		t.Fatalf("stringify %v failed: %v", path.ValueFilter, err)
	}
	text := filter.AttrPath{URI: path.URI, Name: path.Name}.String() + "[" + valueFilter + "]"
	if path.SubAttribute != "" {
		text += "." + path.SubAttribute
	}
	return text
}

func checkRoundTrip(t *testing.T, g filter.Grammar, input string) {
	t.Helper()
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

	text, err := filter.Visit[string](filter.Stringify{}, node)
	if err != nil {
		t.Fatalf("stringify %q failed: %v", input, err)
	}
	reparsed, err := g.Parse(text)
	if err != nil {
		t.Fatalf("stringified %q -> %q failed to reparse: %v", input, text, err)
	}
	if !reflect.DeepEqual(node, reparsed) {
		t.Fatalf("round-trip changed the AST: %q -> %q", input, text)
	}
}
