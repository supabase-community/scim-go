package server

import (
	"context"
	"slices"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

const maxListCount = 1 << 30

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
			return cloneResource(item)
		}
	}
	var zero T
	return zero, scimerrors.ErrNotFound("resource " + id + " not found")
}

func (r *MemoryRepository[T]) List(_ context.Context, query *protocol.SearchRequest) ([]T, int, error) {
	if query.Filter != "" {
		if _, err := matchesFilter(r.schemas, query.Filter, map[string]any{}); err != nil {
			return nil, 0, err
		}
	}

	matched := make([]T, 0, len(r.items))
	for _, item := range r.items {
		if query.Filter != "" {
			ok, err := matchesFilter(r.schemas, query.Filter, item)
			if err != nil {
				return nil, 0, err
			}
			if !ok {
				continue
			}
		}
		matched = append(matched, item)
	}
	sortItems(matched, query.SortBy, query.Descending())

	total := len(matched)
	start := min(query.Offset(), total)
	end := min(start+query.Count, total)

	page := make([]T, end-start)
	for i, item := range matched[start:end] {
		clone, err := cloneResource(item)
		if err != nil {
			return nil, 0, err
		}
		page[i] = clone
	}
	return page, total, nil
}

func (r *MemoryRepository[T]) Create(_ context.Context, item T) (T, error) {
	stored, err := cloneResource(item)
	if err != nil {
		var zero T
		return zero, err
	}
	r.items = append(r.items, stored)
	return cloneResource(stored)
}

func (r *MemoryRepository[T]) Replace(_ context.Context, id string, item T) (T, error) {
	for i, existing := range r.items {
		if existing.ResourceID() == id {
			stored, err := cloneResource(item)
			if err != nil {
				var zero T
				return zero, err
			}
			r.items[i] = stored
			return cloneResource(stored)
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
