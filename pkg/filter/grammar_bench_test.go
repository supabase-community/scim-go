package filter

import (
	"strconv"
	"strings"
	"testing"
)

func chain(op string, n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "a" + strconv.Itoa(i) + ` eq "` + strconv.Itoa(i) + `"`
	}
	return strings.Join(parts, " "+op+" ")
}

func BenchmarkParseLeaf(b *testing.B) {
	g := &Grammar{}
	input := `userName eq "bjensen"`
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := g.Parse(input); err != nil {
			b.Fatal("parse failed")
		}
	}
}

func BenchmarkParseAndChain(b *testing.B) {
	g := &Grammar{}
	input := chain("and", 20)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := g.Parse(input); err != nil {
			b.Fatal("parse failed")
		}
	}
}

func BenchmarkParseOrChain(b *testing.B) {
	g := &Grammar{}
	input := chain("or", 20)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := g.Parse(input); err != nil {
			b.Fatal("parse failed")
		}
	}
}

func BenchmarkParseNestedParens(b *testing.B) {
	g := &Grammar{}
	const depth = 20
	input := strings.Repeat("(", depth) + `userName eq "bjensen"` + strings.Repeat(")", depth)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := g.Parse(input); err != nil {
			b.Fatal("parse failed")
		}
	}
}
