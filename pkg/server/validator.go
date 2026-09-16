package server

import (
	"context"

	"github.com/supabase-community/scim-go/pkg/core"
)

// Validator is a business-rule hook DefaultService runs before persisting a
// Create or Replace, in addition to schema validation. excludeID is empty on
// Create and the resource's own id on Replace, so a Validator can exclude the
// resource being replaced from checks like uniqueness.
type Validator[T core.Resource] func(ctx context.Context, candidate T, excludeID string) error
