package peg_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/filter/internal/peg"
)

func TestRefResolvesLateBoundParser(t *testing.T) {
	var p peg.Parser
	ref := peg.Ref(&p)
	p = peg.Str("x")

	val, err := ref(peg.NewContext("x"))

	require.NoError(t, err)
	assert.Equal(t, "x", val)
}
