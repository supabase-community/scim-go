package scim

import (
	"context"
	"errors"

	"github.com/supabase-community/go-scim/pkg/core"
	"github.com/supabase-community/go-scim/pkg/protocol"
)

type Repository[T core.Resource] interface {
	Get(ctx context.Context, id string) (T, error)
	List(ctx context.Context, query *protocol.SearchRequest) (items []T, total int, err error)
	Create(ctx context.Context, item T) (T, error)
	Replace(ctx context.Context, id string, item T) (T, error)
	Delete(ctx context.Context, id string) error
}

var ErrNotFound = errors.New("scim: resource not found")
