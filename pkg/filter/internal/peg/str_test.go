package peg

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStr(t *testing.T) {
	tt := []struct {
		stream string
		input  string

		position int
		ok       bool
		result   any
	}{
		{stream: "abcd ", input: "abcd", position: 4, ok: true, result: "abcd"},
		{stream: "   abcd ", input: "ab", position: 0, ok: false, result: nil},
		{stream: " cd ", input: "ab", position: 0, ok: false, result: nil},
		{stream: "ABCD ", input: "abcd", position: 0, ok: false, result: nil},
	}
	for _, expected := range tt {
		t.Run(fmt.Sprintf("%s %d", expected.stream, expected.position), func(t *testing.T) {
			atom := Str(expected.input)
			ctx := NewContext(expected.stream)
			result, err := atom(ctx)

			assert.Equal(t, expected.ok, err == nil)
			assert.Equal(t, expected.position, ctx.position)
			assert.Equal(t, expected.result, result)
		})
	}
}
