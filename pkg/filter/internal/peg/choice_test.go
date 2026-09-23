package peg_test

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/filter/internal/peg"
)

func TestChoice(t *testing.T) {
	atom := peg.Sequence(
		peg.Str("userName"),
		peg.Space(),
		peg.Choice(
			peg.Str("eq"),
			peg.Str("ne"),
			peg.Str("co"),
			peg.Str("sw"),
			peg.Str("ew"),
			peg.Str("gt"),
			peg.Str("ge"),
			peg.Str("lt"),
			peg.Str("le"),
		),
		peg.Space(),
		peg.Match(regexp.MustCompile(`"[a-z]+"`)),
	)

	tt := []struct {
		stream string
		input  string

		position int
		ok       bool
		result   any
	}{
		{stream: `userName eq "bjensen"`, position: 21, ok: true, result: []peg.ASTNode{"userName", "eq", `"bjensen"`}},
		{stream: `userName ne "bjensen"`, position: 21, ok: true, result: []peg.ASTNode{"userName", "ne", `"bjensen"`}},
		{stream: `userName co "jen"`, position: 17, ok: true, result: []peg.ASTNode{"userName", "co", `"jen"`}},
		{stream: `userName sw "bjen"`, position: 18, ok: true, result: []peg.ASTNode{"userName", "sw", `"bjen"`}},
		{stream: `userName ew "sen"`, position: 17, ok: true, result: []peg.ASTNode{"userName", "ew", `"sen"`}},
		{stream: `userName gt "zero"`, position: 18, ok: true, result: []peg.ASTNode{"userName", "gt", `"zero"`}},
		{stream: `userName ge "zero"`, position: 18, ok: true, result: []peg.ASTNode{"userName", "ge", `"zero"`}},
		{stream: `userName lt "zero"`, position: 18, ok: true, result: []peg.ASTNode{"userName", "lt", `"zero"`}},
		{stream: `userName le "zero"`, position: 18, ok: true, result: []peg.ASTNode{"userName", "le", `"zero"`}},
	}
	for _, expected := range tt {
		t.Run(fmt.Sprintf("%s %d", expected.stream, expected.position), func(t *testing.T) {
			ctx := peg.NewContext(expected.stream)
			result, err := atom(ctx)

			assert.Equal(t, expected.ok, err == nil)
			assert.Equal(t, expected.position, ctx.Position())
			assert.Equal(t, expected.result, result)
		})
	}
}

func TestChoiceNoMatch(t *testing.T) {
	atom := peg.Choice(
		peg.Str("eq"),
		peg.Str("ne"),
		peg.Str("co"),
	)

	tt := []struct {
		stream string
	}{
		{stream: "xx"},
		{stream: ""},
		{stream: "abc"},
	}
	for _, expected := range tt {
		t.Run(expected.stream, func(t *testing.T) {
			ctx := peg.NewContext(expected.stream)
			result, err := atom(ctx)

			require.Error(t, err)
			assert.Nil(t, result)
			assert.Equal(t, 0, ctx.Position())
		})
	}
}
