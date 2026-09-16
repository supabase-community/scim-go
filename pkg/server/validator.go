package server

import (
	"context"

	"github.com/supabase-community/scim-go/pkg/core"
)

type Validator[T core.Resource] func(ctx context.Context, candidate T) error
