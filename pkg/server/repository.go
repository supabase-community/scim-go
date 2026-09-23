package server

import (
	"context"
	"slices"
	"strconv"
	"sync"
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
	Delete(ctx context.Context, id, version string) error
}

type repository[T Entity] struct {
	mu       sync.Mutex
	endpoint string
	schema   *core.Schema
	items    []T
	readers[T]
	evaluator protocol.Evaluator[predicate]
}

// NewRepository stores resources in memory, for tests and reference servers.
func NewRepository[T Entity](endpoint string, schema *core.Schema, fields Fields[T]) Repository[T] {
	readers := fields.readers()
	return &repository[T]{
		endpoint:  endpoint,
		schema:    schema,
		items:     []T{},
		readers:   readers,
		evaluator: newVisitor[T](readers),
	}
}

func (r *repository[T]) Get(_ context.Context, id string) (item T, err error) {
	r.withLock(func() {
		var i int
		if i, err = r.locate(id, ""); err == nil {
			item = r.items[i]
		}
	})
	return item, err
}

func (r *repository[T]) List(_ context.Context, query *protocol.SearchRequest) ([]T, int, error) {
	var matching []T
	var err error
	r.withLock(func() { matching, err = r.sortBy(query) })
	if err != nil {
		return []T{}, 0, err
	}

	total := len(matching)
	start := min(query.Offset(), total)
	end := min(start+query.Count, total)
	return matching[start:end], total, nil
}

func (r *repository[T]) Create(_ context.Context, item T) (T, error) {
	now := time.Now().UTC()
	id := uuid.NewV7().String()
	item.SetID(id)
	item.SetSchemas([]core.SchemaURI{r.schema.ID})
	item.SetMeta(core.Meta{
		ResourceType: r.schema.Name,
		Created:      now,
		LastModified: now,
		Location:     r.endpoint + "/" + id,
		Version:      weakETag(now),
	})

	r.withLock(func() { r.items = append(r.items, item) })
	return item, nil
}

func (r *repository[T]) Replace(_ context.Context, item T) (T, error) {
	var err error
	r.withLock(func() {
		var i int
		if i, err = r.locate(item.ResourceID(), item.GetMeta().Version); err != nil {
			return
		}
		meta := r.items[i].GetMeta()
		now := time.Now().UTC()
		meta.LastModified = now
		meta.Version = weakETag(now)
		item.SetMeta(meta)
		item.SetSchemas([]core.SchemaURI{r.schema.ID})
		r.items[i] = item
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return item, nil
}

func (r *repository[T]) Delete(_ context.Context, id, version string) (err error) {
	r.withLock(func() {
		var i int
		if i, err = r.locate(id, version); err == nil {
			r.items = slices.Delete(r.items, i, i+1)
		}
	})
	return err
}

func (r *repository[T]) withLock(fn func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn()
}

// RFC 7644 Section 3.14: a non-empty version must match the stored one.
func (r *repository[T]) locate(id, version string) (int, error) {
	i := slices.IndexFunc(r.items, func(item T) bool { return item.ResourceID() == id })
	switch {
	case i < 0:
		return i, scimerrors.ErrNotFound("Not found")
	case version != "" && r.items[i].GetMeta().Version != version:
		return i, scimerrors.ErrPreconditionFailed("resource has changed on the server")
	}
	return i, nil
}

func (r *repository[T]) filterBy(query *protocol.SearchRequest) ([]T, error) {
	if query.Filter == "" {
		return slices.Clone(r.items), nil
	}

	predicate, err := protocol.Filter([]*core.Schema{r.schema}, query.Filter, r.evaluator)
	if err != nil {
		return []T{}, err
	}
	matching := []T{}
	for _, item := range r.items {
		if predicate(item) {
			matching = append(matching, item)
		}
	}
	return matching, nil
}

func weakETag(t time.Time) string {
	return `W/"` + strconv.FormatInt(t.UnixNano(), 10) + `"`
}
