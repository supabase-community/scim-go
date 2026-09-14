package core_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/supabase-community/scim-go/pkg/core"
)

func TestResourceIDAccessors(t *testing.T) {
	t.Run("Group returns its id", func(t *testing.T) {
		g := &core.Group{ID: "g-1"}
		assert.Equal(t, "g-1", g.ResourceID())
	})

	t.Run("User returns its id", func(t *testing.T) {
		u := &core.User{ID: "u-1"}
		assert.Equal(t, "u-1", u.ResourceID())
	})

	t.Run("Schema returns its id", func(t *testing.T) {
		s := &core.Schema{ID: core.SchemaUser}
		assert.Equal(t, string(core.SchemaUser), s.ResourceID())
	})

	t.Run("ResourceType returns its id", func(t *testing.T) {
		rt := &core.ResourceType{ID: core.ResourceTypeName("User")}
		assert.Equal(t, "User", rt.ResourceID())
	})
}

func TestSchemaDescribe(t *testing.T) {
	s := &core.Schema{ID: core.SchemaUser}
	assert.Same(t, s, s.Describe("the user resource"))
	assert.Equal(t, "the user resource", s.Description)
}

func TestResourceTypeNameReference(t *testing.T) {
	assert.Equal(t, core.ReferenceType("User"), core.ResourceTypeName("User").Reference())
}
