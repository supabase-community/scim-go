package core_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/supabase-community/scim-go/pkg/core"
)

func TestBase(t *testing.T) {
	t.Run("Common returns the common attributes of a resource", func(t *testing.T) {
		user := &core.User{ID: "u-1"}
		user.Common().ExternalID = "ext-1"

		assert.Equal(t, "u-1", user.Common().ID)
		assert.Equal(t, "ext-1", user.ExternalID)
	})
}
