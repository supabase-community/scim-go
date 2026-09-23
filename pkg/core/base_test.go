package core_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/supabase-community/scim-go/pkg/core"
)

func TestBase(t *testing.T) {
	t.Run("SetID sets the id", func(t *testing.T) {
		var base core.Base
		base.SetID("2819c223")
		assert.Equal(t, "2819c223", base.ID)
	})

	t.Run("SetSchemas sets the schemas list", func(t *testing.T) {
		var base core.Base
		base.SetSchemas([]core.SchemaURI{core.SchemaUser})
		assert.Equal(t, []core.SchemaURI{core.SchemaUser}, base.Schemas)
	})

	t.Run("SetMeta and GetMeta round-trip", func(t *testing.T) {
		var base core.Base
		meta := core.Meta{ResourceType: "User", Version: `W/"1"`}
		base.SetMeta(meta)
		assert.Equal(t, meta, base.GetMeta())
	})

	t.Run("GetExternalID returns the externalId", func(t *testing.T) {
		base := core.Base{ExternalID: "ext-1"}
		assert.Equal(t, "ext-1", base.GetExternalID())
	})
}
