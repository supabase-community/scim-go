package peg

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChoice(t *testing.T) {
	atom := Sequence(
		Str("userName"),
		Space(),
		Choice(
			Str("eq"),
			Str("ne"),
			Str("co"),
			Str("sw"),
			Str("ew"),
			Str("gt"),
			Str("ge"),
			Str("lt"),
			Str("le"),
		),
		Space(),
		Match(regexp.MustCompile(`"[a-z]+"`)),
	)

	tt := []struct {
		stream string
		input  string

		position int
		ok       bool
		result   any
	}{
		{stream: `userName eq "bjensen"`, position: 21, ok: true, result: []ASTNode{"userName", "eq", `"bjensen"`}},
		{stream: `userName ne "bjensen"`, position: 21, ok: true, result: []ASTNode{"userName", "ne", `"bjensen"`}},
		{stream: `userName co "jen"`, position: 17, ok: true, result: []ASTNode{"userName", "co", `"jen"`}},
		{stream: `userName sw "bjen"`, position: 18, ok: true, result: []ASTNode{"userName", "sw", `"bjen"`}},
		{stream: `userName ew "sen"`, position: 17, ok: true, result: []ASTNode{"userName", "ew", `"sen"`}},
		{stream: `userName gt "zero"`, position: 18, ok: true, result: []ASTNode{"userName", "gt", `"zero"`}},
		{stream: `userName ge "zero"`, position: 18, ok: true, result: []ASTNode{"userName", "ge", `"zero"`}},
		{stream: `userName lt "zero"`, position: 18, ok: true, result: []ASTNode{"userName", "lt", `"zero"`}},
		{stream: `userName le "zero"`, position: 18, ok: true, result: []ASTNode{"userName", "le", `"zero"`}},
	}
	for _, expected := range tt {
		t.Run(fmt.Sprintf("%s %d", expected.stream, expected.position), func(t *testing.T) {
			ctx := NewContext(expected.stream)
			result, err := atom(ctx)

			assert.Equal(t, expected.ok, err == nil)
			assert.Equal(t, expected.position, ctx.position)
			assert.Equal(t, expected.result, result)
		})
	}
}

func TestChoiceNoMatch(t *testing.T) {
	atom := Choice(
		Str("eq"),
		Str("ne"),
		Str("co"),
	)

	tt := []struct {
		stream string
	}{
		{stream: "xx"},
		{stream: ""},
		{stream: "abc"},
	}
	for _, expected := range tt {
		t.Run(fmt.Sprintf("%s", expected.stream), func(t *testing.T) {
			ctx := &Context{stream: expected.stream}
			result, err := atom(ctx)

			assert.Error(t, err)
			assert.Nil(t, result)
			assert.Equal(t, 0, ctx.position)
		})
	}
}
