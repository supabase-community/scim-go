package peg_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/supabase-community/scim-go/pkg/filter/internal/peg"
)

func TestFold(t *testing.T) {
	tt := []struct {
		stream string
		input  string

		position int
		ok       bool
		result   any
	}{
		{stream: "and rest", input: "and", position: 3, ok: true, result: "and"},
		{stream: "AND rest", input: "and", position: 3, ok: true, result: "and"},
		{stream: "And", input: "and", position: 3, ok: true, result: "and"},
		{stream: "or", input: "and", position: 0, ok: false, result: nil},
		{stream: "an", input: "and", position: 0, ok: false, result: nil},
	}
	for _, expected := range tt {
		t.Run(fmt.Sprintf("%s %s", expected.stream, expected.input), func(t *testing.T) {
			atom := peg.Fold(expected.input)
			ctx := peg.NewContext(expected.stream)
			result, err := atom(ctx)

			assert.Equal(t, expected.ok, err == nil)
			assert.Equal(t, expected.position, ctx.Position())
			assert.Equal(t, expected.result, result)
		})
	}
}
