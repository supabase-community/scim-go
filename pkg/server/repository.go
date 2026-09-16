package server

import (
	"context"
	"slices"
	"strconv"
	"time"
	"uuid"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type Repository[T Entity] interface {
	List(ctx context.Context, query *protocol.SearchRequest) (items []T, total int, err error)
	Get(ctx context.Context, id string) (T, error)
	Create(ctx context.Context, item T) (T, error)
	Replace(ctx context.Context, id string, item T) (T, error)
	Delete(ctx context.Context, id string) error
}

type repository[T Entity] struct {
	schema *core.Schema
	items  []T
}

func NewRepository[T Entity](schemas *core.Schema) Repository[T] {
	return &repository[T]{schema: schemas}
}

func (r *repository[T]) Get(_ context.Context, id string) (T, error) {
	for _, item := range r.items {
		if item.ResourceID() == id {
			return item, nil
		}
	}
	var zero T
	return zero, scimerrors.ErrNotFound("resource " + id + " not found")
}

func (r *repository[T]) List(_ context.Context, query *protocol.SearchRequest) ([]T, int, error) {
	total := len(r.items)
	start := min(query.Offset(), total)
	end := min(start+query.Count, total)
	page := r.items[start:end]
	return page, total, nil
}

func (r *repository[T]) Create(_ context.Context, item T) (T, error) {
	now := time.Now().UTC()
	item.SetID(uuid.NewV7().String())
	item.SetMeta(core.Meta{
		ResourceType: r.schema.Name,
		Created:      now,
		LastModified: now,
		Version:      strconv.FormatInt(now.Unix(), 10),
	})

	r.items = append(r.items, item)
	return item, nil
}

func (r *repository[T]) Replace(ctx context.Context, id string, item T) (T, error) {
	for i, existing := range r.items {
		if existing.ResourceID() == id {
			meta := existing.GetMeta()
			now := time.Now().UTC()
			meta.LastModified = now
			meta.Version = strconv.FormatInt(now.Unix(), 10)
			item.SetMeta(meta)

			r.items[i] = item
			return item, nil
		}
	}
	var zero T
	return zero, scimerrors.ErrNotFound("resource " + id + " not found")
}

func (r *repository[T]) Delete(_ context.Context, id string) error {
	for i, existing := range r.items {
		if existing.ResourceID() == id {
			r.items = slices.Delete(r.items, i, i+1)
			return nil
		}
	}
	return scimerrors.ErrNotFound("resource " + id + " not found")
}
