package protocol

import "github.com/supabase-community/scim-go/pkg/filter"

type valueWrite struct {
	path       filter.Path
	value      any
	appendMode bool
}
