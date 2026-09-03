package filter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNodeAttrPathSplitsParts(t *testing.T) {
	tt := []struct {
		input string
		want  AttrPath
	}{
		{`userName eq "x"`, AttrPath{Name: "userName"}},
		{`name.familyName eq "x"`, AttrPath{Name: "name", Sub: "familyName"}},
		{
			`urn:ietf:params:scim:schemas:core:2.0:User:userName eq "x"`, AttrPath{URI: "urn:ietf:params:scim:schemas:core:2.0:User", Name: "userName"},
		},
		{
			`urn:ietf:params:scim:schemas:core:2.0:User:name.familyName eq "x"`, AttrPath{URI: "urn:ietf:params:scim:schemas:core:2.0:User", Name: "name", Sub: "familyName"},
		},
	}
	g := &Grammar{}
	for _, tc := range tt {
		t.Run(tc.input, func(t *testing.T) {
			node, err := g.Parse(tc.input)
			require.NoError(t, err)

			got := node.AttrPath()

			assert.Equal(t, tc.want, got)
			assert.Equal(t, node.Attribute(), got.String())
		})
	}
}

func TestNodeAttrPathOfValuePath(t *testing.T) {
	g := &Grammar{}
	node, err := g.Parse(`emails[type eq "work"]`)
	require.NoError(t, err)

	assert.Equal(t, AttrPath{Name: "emails"}, node.AttrPath())
}
