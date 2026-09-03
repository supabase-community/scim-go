package peg

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpace(t *testing.T) {
	atom := Space()

	tt := []struct {
		stream   string
		position int
	}{
		{stream: "   abcd ", position: 3},
		{stream: "   abcd", position: 3},
		{stream: " ", position: 1},
		{stream: "", position: 0},
		{stream: "A", position: 0},
		{stream: "ABCD", position: 0},
		{stream: "abcd", position: 0},
	}
	for _, expected := range tt {
		t.Run(fmt.Sprintf("%s %d", expected.stream, expected.position), func(t *testing.T) {
			ctx := NewContext(expected.stream)
			item, err := atom(ctx)

			require.Nil(t, item)
			assert.Equal(t, expected.position, ctx.position)
			assert.NoError(t, err)
		})
	}
}
