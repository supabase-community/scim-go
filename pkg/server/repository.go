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

// RFC 7643 3.1 / RFC 7644 3.14 - Create/Replace stamp id+meta; Replace/Delete honour a non-empty expected version.
type Repository[T Entity] interface {
	List(ctx context.Context, query *protocol.SearchRequest) (items []T, total int, err error)
	Get(ctx context.Context, id string) (T, error)
	Create(ctx context.Context, item T) (T, error)
	Replace(ctx context.Context, item T) (T, error)
	Delete(ctx context.Context, id string, version string) error
}

type repository[T Entity] struct {
	schema    *core.Schema
	items     []T
	evaluator protocol.Evaluator[specification[T]]
}

func NewRepository[T Entity](schemas *core.Schema, evaluator protocol.Evaluator[specification[T]]) Repository[T] {
	return &repository[T]{
		schema:    schemas,
		items:     []T{},
		evaluator: evaluator,
	}
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
	matching := r.items

	if query.Filter != "" {
		predicate, err := protocol.Filter([]*core.Schema{r.schema}, query.Filter, r.evaluator)
		if err != nil {
			return []T{}, 0, err
		}
		matching = []T{}
		for _, item := range r.items {
			if predicate(item) {
				matching = append(matching, item)
			}
		}
	}

	total := len(matching)
	start := min(query.Offset(), total)
	end := min(start+query.Count, total)
	page := matching[start:end]
	return page, total, nil
}

func (r *repository[T]) Create(_ context.Context, item T) (T, error) {
	now := time.Now().UTC()
	item.SetID(uuid.NewV7().String())
	item.SetSchemas([]core.SchemaURI{r.schema.ID})
	item.SetMeta(core.Meta{
		ResourceType: r.schema.Name,
		Created:      now,
		LastModified: now,
		Version:      weakETag(now),
	})

	r.items = append(r.items, item)
	return item, nil
}

func (r *repository[T]) Replace(ctx context.Context, item T) (T, error) {
	id := item.ResourceID()
	version := item.GetMeta().Version
	for i, existing := range r.items {
		if existing.ResourceID() == id {
			if version != "" && existing.GetMeta().Version != version {
				var zero T
				return zero, scimerrors.ErrPreconditionFailed("resource " + id + " has changed on the server")
			}
			meta := existing.GetMeta()
			now := time.Now().UTC()
			meta.LastModified = now
			meta.Version = weakETag(now)
			item.SetMeta(meta)
			item.SetSchemas([]core.SchemaURI{r.schema.ID})

			r.items[i] = item
			return item, nil
		}
	}
	var zero T
	return zero, scimerrors.ErrNotFound("resource " + id + " not found")
}

func (r *repository[T]) Delete(_ context.Context, id string, version string) error {
	for i, existing := range r.items {
		if existing.ResourceID() == id {
			if version != "" && existing.GetMeta().Version != version {
				return scimerrors.ErrPreconditionFailed("resource " + id + " has changed on the server")
			}
			r.items = slices.Delete(r.items, i, i+1)
			return nil
		}
	}
	return scimerrors.ErrNotFound("resource " + id + " not found")
}

// weakETag formats a version per RFC 7644 3.14's worked example: a quoted weak entity-tag.
func weakETag(t time.Time) string {
	return `W/"` + strconv.FormatInt(t.Unix(), 10) + `"`
}
