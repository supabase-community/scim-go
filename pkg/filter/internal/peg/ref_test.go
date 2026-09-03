package peg

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRefResolvesLateBoundParser(t *testing.T) {
	var p Parser
	ref := Ref(&p)
	p = Str("x")

	val, err := ref(NewContext("x"))

	require.NoError(t, err)
	assert.Equal(t, "x", val)
}
