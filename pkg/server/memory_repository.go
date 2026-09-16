package server

import (
	"context"
	"slices"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type MemoryRepository[T core.Resource] struct {
	schemas []*core.Schema
	items   []T
}

func NewMemoryRepository[T core.Resource](schemas []*core.Schema) *MemoryRepository[T] {
	return &MemoryRepository[T]{schemas: schemas}
}

func (r *MemoryRepository[T]) Get(_ context.Context, id string) (T, error) {
	for _, item := range r.items {
		if item.ResourceID() == id {
			return item, nil
		}
	}
	var zero T
	return zero, scimerrors.ErrNotFound("resource " + id + " not found")
}

func (r *MemoryRepository[T]) List(_ context.Context, query *protocol.SearchRequest) ([]T, int, error) {
	total := len(r.items)
	start := min(query.Offset(), total)
	end := min(start+query.Count, total)
	page := r.items[start:end]
	return page, total, nil
}

func (r *MemoryRepository[T]) Create(_ context.Context, item T) (T, error) {
	r.items = append(r.items, item)
	return item, nil
}

func (r *MemoryRepository[T]) Replace(_ context.Context, id string, item T) (T, error) {
	for i, existing := range r.items {
		if existing.ResourceID() == id {
			r.items[i] = item
			return item, nil
		}
	}
	var zero T
	return zero, scimerrors.ErrNotFound("resource " + id + " not found")
}

func (r *MemoryRepository[T]) Delete(_ context.Context, id string) error {
	for i, existing := range r.items {
		if existing.ResourceID() == id {
			r.items = slices.Delete(r.items, i, i+1)
			return nil
		}
	}
	return scimerrors.ErrNotFound("resource " + id + " not found")
}
