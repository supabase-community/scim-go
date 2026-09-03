package peg

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
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
			atom := Fold(expected.input)
			ctx := NewContext(expected.stream)
			result, err := atom(ctx)

			assert.Equal(t, expected.ok, err == nil)
			assert.Equal(t, expected.position, ctx.position)
			assert.Equal(t, expected.result, result)
		})
	}
}
