package server

import (
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

// Limits adds the PATCH budgets that zero lifts.
type Limits struct {
	protocol.Limits
	MaxFilterEvaluations int
	MaxWriteBytes        int
}

var DefaultLimits = Limits{
	Limits:               protocol.DefaultLimits,
	MaxFilterEvaluations: 10_000_000,
	MaxWriteBytes:        patch.DefaultMaxWriteBytes,
}
