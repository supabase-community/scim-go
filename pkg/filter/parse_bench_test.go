package filter_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/supabase-community/scim-go/pkg/filter"
)

func BenchmarkParse(b *testing.B) {
	const size = 20
	cases := []struct {
		name  string
		input string
	}{
		{"leaf", `userName eq "bjensen"`},
		{"and_chain", chain("and", size)},
		{"or_chain", chain("or", size)},
		{"nested_parens", strings.Repeat("(", size) + `userName eq "bjensen"` + strings.Repeat(")", size)},
	}
	for _, c := range cases {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := filter.Parse(c.input); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func chain(op string, n int) string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "a" + strconv.Itoa(i) + ` eq "` + strconv.Itoa(i) + `"`
	}
	return strings.Join(parts, " "+op+" ")
}
