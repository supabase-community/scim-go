package server

import (
	"context"
	"slices"
	"time"
	"uuid"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
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
	db       *database
	endpoint string
	schemas  core.Schemas
	fields
	evaluator protocol.Evaluator[predicate]
}

// NewRepository stores resources in memory, for tests and reference servers.
func NewRepository[T core.Resource](endpoint string, schemas core.Schemas) Repository[T] {
	return newRepository[T](newDatabase(), endpoint, schemas)
}

func newRepository[T core.Resource](db *database, endpoint string, schemas core.Schemas) Repository[T] {
	fields := newFields(schemas)
	return &repository[T]{
		db:        db,
		endpoint:  endpoint,
		schemas:   schemas,
		fields:    fields,
		evaluator: newEvaluator(fields),
	}
}

func (r *repository[T]) Read(_ context.Context, id string) (T, error) {
	item, err := r.db.find(id, r.schemas)
	if err != nil {
		var zero T
		return zero, err
	}
	return cast[T](item)
}

func (r *repository[T]) List(_ context.Context, query *protocol.SearchRequest) ([]T, int, error) {
	if query == nil {
		query = &protocol.SearchRequest{}
	}
	matching, err := r.sortBy(query, r.db.all(r.schemas))
	if err != nil {
		return []T{}, 0, err
	}
	total := len(matching)
	start := min(query.Offset(), total)
	end := start + min(max(query.Count, 0), total-start)
	items := make([]T, 0, end-start)
	for _, rec := range matching[start:end] {
		copied, err := cast[T](clone(rec))
		if err != nil {
			return []T{}, 0, err
		}
		items = append(items, copied)
	}
	return items, total, nil
}

func (r *repository[T]) Create(_ context.Context, item T) (T, error) {
	at := now()
	common := item.Common()
	common.ID = uuid.NewV7().String()
	common.Meta = core.Meta{
		ResourceType: r.schemas[0].Name,
		Created:      at,
		LastModified: at,
		Location:     r.endpoint + "/" + common.ID,
		Version:      WeakETag(at),
	}
	created, err := r.db.create(item, r.schemas, r.fields)
	if err != nil {
		var zero T
		return zero, err
	}
	return cast[T](created)
}

func (r *repository[T]) Update(_ context.Context, item T) (T, error) {
	updated, err := r.db.update(item, r.schemas, r.fields)
	if err != nil {
		var zero T
		return zero, err
	}
	return cast[T](updated)
}

func (r *repository[T]) Delete(_ context.Context, item T) error {
	common := item.Common()
	return r.db.remove(common.ID, common.Meta.Version, r.schemas, r.fields)
}

func (r *repository[T]) filterBy(query *protocol.SearchRequest, records []record) ([]record, error) {
	if query.Filter == "" {
		return records, nil
	}

	predicate, err := protocol.Filter(r.schemas, query.Filter, r.evaluator)
	if err != nil {
		return nil, err
	}
	matching := []record{}
	for _, rec := range records {
		if predicate(rec.object) {
			matching = append(matching, rec)
		}
	}
	return matching, nil
}

func (r *repository[T]) sortBy(query *protocol.SearchRequest, records []record) ([]record, error) {
	matching, err := r.filterBy(query, records)
	if err != nil {
		return nil, err
	}
	if query.SortBy == "" {
		return matching, nil
	}

	sorted, err := query.SortAttribute(r.schemas)
	if err != nil {
		return nil, err
	}

	key, ok := r.sortKey(sorted.Root(), sorted.Definition)
	if !ok {
		return nil, scimerrors.ErrInvalidValue("Unknown sortBy")
	}
	slices.SortStableFunc(matching, func(a, b record) int {
		return compareSortKeys(sorted.Definition, key(a.object), key(b.object), query.Descending())
	})

	return matching, nil
}

// RFC 7644 Section 3.4.2.3: a multi-valued attribute sorts by its primary value, or else its first value.
func (r *repository[T]) sortKey(parent, attribute *core.Attribute) (func(core.Object) any, bool) {
	list, ok := r.lookup(parent)
	if !ok || !list.isList() {
		field, ok := r.lookup(attribute)
		return field.value, ok
	}
	return func(d core.Object) any {
		element, ok := primaryOrFirst(list.elements(d))
		if !ok {
			return nil
		}
		return coerce(attribute, value.AsObject(element).Get(attribute.Name))
	}, true
}

func now() time.Time {
	return time.Now().UTC().Truncate(time.Microsecond)
}

func bumpMeta(current core.Meta) core.Meta {
	modified := now()
	if !modified.After(current.LastModified) {
		modified = current.LastModified.Add(time.Microsecond)
	}
	current.LastModified = modified
	current.Version = WeakETag(modified)
	return current
}

func cast[T any](v any) (T, error) {
	got, ok := v.(T)
	if !ok {
		return got, scimerrors.ErrInternal("stored resource has an unexpected type")
	}
	return got, nil
}
