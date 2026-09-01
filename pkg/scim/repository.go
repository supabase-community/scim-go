// Package scim implements the SCIM 2.0 protocol defined in RFC 7644.
package scim

import (
	"context"
	"errors"

	"mokhan.ca/go/scim/pkg/core"
	"mokhan.ca/go/scim/pkg/protocol"
)

type Repository[T core.Resource] interface {
	Get(ctx context.Context, id string) (T, error)
	List(ctx context.Context, query *protocol.SearchRequest) (items []T, total int, err error)
	Create(ctx context.Context, item T) (T, error)
	Replace(ctx context.Context, id string, item T) (T, error)
	Delete(ctx context.Context, id string) error
}

var ErrNotFound = errors.New("scim: resource not found")
