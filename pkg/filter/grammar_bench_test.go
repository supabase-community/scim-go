package filter_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/supabase-community/scim-go/pkg/filter"
)

func BenchmarkParseLeaf(b *testing.B) {
	input := `userName eq "bjensen"`
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := filter.Parse(input); err != nil {
			b.Fatal("parse failed")
		}
	}
}

func BenchmarkParseAndChain(b *testing.B) {
	input := chain("and", 20)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := filter.Parse(input); err != nil {
			b.Fatal("parse failed")
		}
	}
}

func BenchmarkParseOrChain(b *testing.B) {
	input := chain("or", 20)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := filter.Parse(input); err != nil {
			b.Fatal("parse failed")
		}
	}
}

func BenchmarkParseNestedParens(b *testing.B) {
	const depth = 20
	input := strings.Repeat("(", depth) + `userName eq "bjensen"` + strings.Repeat(")", depth)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := filter.Parse(input); err != nil {
			b.Fatal("parse failed")
		}
	}
}

func chain(op string, n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "a" + strconv.Itoa(i) + ` eq "` + strconv.Itoa(i) + `"`
	}
	return strings.Join(parts, " "+op+" ")
}
