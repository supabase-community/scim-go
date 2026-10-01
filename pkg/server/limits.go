package server

import (
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

// Limits adds the PATCH budgets and MaxResourceBytes cap that zero lifts.
type Limits struct {
	protocol.Limits
	MaxFilterEvaluations int
	MaxWriteBytes        int
	MaxResourceBytes     int
}

var DefaultLimits = Limits{
	Limits:               protocol.DefaultLimits,
	MaxFilterEvaluations: 10_000_000,
	MaxWriteBytes:        patch.DefaultMaxWriteBytes,
	MaxResourceBytes:     10 << 20,
}
