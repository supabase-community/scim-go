package server

import (
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

// Limits adds the PATCH budgets and MaxResourceBytes cap that zero lifts, and the PatchRetries of a versionless PATCH under RFC 7644 Section 3.14, where zero means none.
type Limits struct {
	protocol.Limits
	MaxFilterEvaluations int
	MaxWriteBytes        int
	MaxResourceBytes     int
	PatchRetries         int
}

var DefaultLimits = Limits{
	Limits:               protocol.DefaultLimits,
	MaxFilterEvaluations: 10_000_000,
	MaxWriteBytes:        patch.DefaultMaxWriteBytes,
	MaxResourceBytes:     10 << 20,
	PatchRetries:         2,
}
