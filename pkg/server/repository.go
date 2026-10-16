package server

import (
	"context"
	"errors"
	"uuid"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
	"github.com/supabase-community/scim-go/pkg/store"
)

// Repository stores resources, enforces uniqueness atomically per RFC 7643 Section 7, and rejects a write whose item.Meta.Version is stale per RFC 7644 Section 3.14.
type Repository[T core.Resource] interface {
	List(ctx context.Context, query *protocol.SearchRequest) (items []T, total int, err error)
	Create(ctx context.Context, item T) (T, error)
	Read(ctx context.Context, id string) (T, error)
	Update(ctx context.Context, item T) (T, error)
	Delete(ctx context.Context, item T) error
}

type repository[T core.Resource] struct {
	db         store.Store
	references []store.Reference
}

func NewRepository[T core.Resource](db store.Store, schemas core.Schemas) Repository[T] {
	return &repository[T]{db: db, references: store.ReferencesOf(schemas)}
}

func (r *repository[T]) List(ctx context.Context, request *protocol.SearchRequest) ([]T, int, error) {
	if request == nil {
		request = &protocol.SearchRequest{}
	}
	records, total, err := r.db.List(ctx, request, r.returned(ctx))
	if err != nil {
		return nil, 0, err
	}
	items, err := store.Decode[T](records)
	return items, total, err
}

func (r *repository[T]) Read(ctx context.Context, id string) (T, error) {
	record, err := r.db.Find(ctx, id, r.returned(ctx))
	if err != nil {
		var zero T
		return zero, err
	}
	return record.Into[T]()
}

func (r *repository[T]) Create(ctx context.Context, item T) (T, error) {
	item.Common().ID = uuid.NewV7().String()
	return r.save(ctx, item, func(doc store.Document) (store.Record, error) {
		return r.db.Create(ctx, doc)
	})
}

func (r *repository[T]) Update(ctx context.Context, item T) (T, error) {
	version := item.Common().Meta.Version
	return r.save(ctx, item, func(doc store.Document) (store.Record, error) {
		record, err := r.db.Update(ctx, doc, version)
		return record, r.mismatch(ctx, doc.ID, err)
	})
}

func (r *repository[T]) Delete(ctx context.Context, item T) error {
	common := item.Common()
	err := r.db.Delete(ctx, common.ID, common.Meta.Version)
	return r.mismatch(ctx, common.ID, err)
}

func (r *repository[T]) save(ctx context.Context, item T, persist func(store.Document) (store.Record, error)) (T, error) {
	var zero T
	doc, err := store.Encode(item, r.references)
	if err != nil {
		return zero, err
	}
	doc.References = r.returned(ctx)
	record, err := persist(doc)
	if err != nil {
		return zero, err
	}
	return record.Into[T]()
}

func (r *repository[T]) returned(ctx context.Context) []store.Reference {
	projection := protocol.ProjectionFrom(ctx)
	returned := make([]store.Reference, 0, len(r.references))
	for _, reference := range r.references {
		if !reference.Soft() && projection.Returns(reference.Name) {
			returned = append(returned, reference)
		}
	}
	return returned
}

func (r *repository[T]) mismatch(ctx context.Context, id string, err error) error {
	if !errors.Is(err, store.ErrNoMatch) {
		return err
	}
	if _, err := r.db.Find(ctx, id, nil); err != nil {
		return err
	}
	return scimerrors.ErrPreconditionFailed("resource has changed on the server")
}
