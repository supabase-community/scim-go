package memory

import "github.com/supabase-community/scim-go/pkg/core"

type row struct {
	id     string
	entry  entry
	object core.Object
}
