package memory

import (
	"encoding/json"
	"time"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/store"
)

type entry struct {
	schema  *core.Schema
	owner   *memoryStore
	body    core.Object
	raw     json.RawMessage
	links   []store.Link
	created time.Time
	updated time.Time
}
