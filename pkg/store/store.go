package store

import (
	"context"
	"errors"

	"github.com/supabase-community/scim-go/pkg/protocol"
)

var ErrNoMatch = errors.New("no resource matches the id and version")

// Store enforces uniqueness (RFC 7643 Section 7), bumps the version of resources at the other end of a changed link (RFC 7232 Section 2.1), and unlinks a deleted resource (RFC 7643 Section 2.3.7). A Record.Body is the full resource JSON with member lists, without id and meta.
type Store interface {
	Find(ctx context.Context, id string, references []Reference) (Record, error)
	List(ctx context.Context, request *protocol.SearchRequest, references []Reference) ([]Record, int, error)
	Create(ctx context.Context, doc Document) (Record, error)
	Update(ctx context.Context, doc Document, version string) (Record, error)
	Delete(ctx context.Context, id, version string) error
}
