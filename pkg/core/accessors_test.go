package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResourceIDAccessors(t *testing.T) {
	t.Run("Group returns its id", func(t *testing.T) {
		g := &Group{ID: "g-1"}
		assert.Equal(t, "g-1", g.ResourceID())
	})

	t.Run("User returns its id", func(t *testing.T) {
		u := &User{ID: "u-1"}
		assert.Equal(t, "u-1", u.ResourceID())
	})

	t.Run("Schema returns its id", func(t *testing.T) {
		s := &Schema{ID: SchemaUser}
		assert.Equal(t, string(SchemaUser), s.ResourceID())
	})

	t.Run("ResourceType returns its id", func(t *testing.T) {
		rt := &ResourceType{ID: ResourceTypeName("User")}
		assert.Equal(t, "User", rt.ResourceID())
	})
}

func TestSchemaDescribe(t *testing.T) {
	s := &Schema{ID: SchemaUser}
	assert.Same(t, s, s.Describe("the user resource"))
	assert.Equal(t, "the user resource", s.Description)
}

func TestResourceTypeNameReference(t *testing.T) {
	assert.Equal(t, ReferenceType("User"), ResourceTypeName("User").Reference())
}
