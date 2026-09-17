package protocol

import "github.com/supabase-community/scim-go/pkg/core"

type Attribute struct {
	*core.Attribute
	Key string
}
