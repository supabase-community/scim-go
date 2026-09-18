package server

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
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
	accessors Accessors[T]
	evaluator protocol.Evaluator[specification[T]]
}

func NewRepository[T Entity](schema *core.Schema, accessors Accessors[T]) Repository[T] {
	return &repository[T]{
		schema:    schema,
		items:     []T{},
		accessors: accessors,
		evaluator: NewVisitor[T](accessors),
	}
}

func (r *repository[T]) Get(_ context.Context, id string) (T, error) {
	for _, item := range r.items {
		if item.ResourceID() == id {
			return item, nil
		}
	}
	var zero T
	return zero, scimerrors.ErrNotFound("Not found")
}

func (r *repository[T]) List(_ context.Context, query *protocol.SearchRequest) ([]T, int, error) {
	matching, err := r.sortBy(query)
	if err != nil {
		return []T{}, 0, err
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
				return zero, scimerrors.ErrPreconditionFailed("resource has changed on the server")
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
	return zero, scimerrors.ErrNotFound("Not found")
}

func (r *repository[T]) Delete(_ context.Context, id string, version string) error {
	for i, existing := range r.items {
		if existing.ResourceID() == id {
			if version != "" && existing.GetMeta().Version != version {
				return scimerrors.ErrPreconditionFailed("resource has changed on the server")
			}
			r.items = slices.Delete(r.items, i, i+1)
			return nil
		}
	}
	return scimerrors.ErrNotFound("Not found")
}

func (r *repository[T]) filterBy(query *protocol.SearchRequest) ([]T, error) {
	if query.Filter == "" {
		return r.items, nil
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

func (r *repository[T]) sortBy(query *protocol.SearchRequest) ([]T, error) {
	matching, err := r.filterBy(query)
	if err != nil {
		return []T{}, err
	}
	if query.SortBy == "" {
		return matching, nil
	}

	path, err := filter.NewAttrPath(query.SortBy)
	if err != nil {
		return []T{}, scimerrors.ErrInvalidValue(err.Error())
	}

	attribute, ok := r.schema.Resolve(path.Name)
	if !ok {
		return []T{}, scimerrors.ErrInvalidValue("Unknown sortBy")
	}

	if path.SubAttribute != "" {
		attribute = attribute.SubAttribute(path.SubAttribute)
	}

	if err != nil {
		return []T{}, err
	}
	accessor, ok := r.accessors[attribute]
	if !ok {
		return []T{}, scimerrors.ErrInvalidValue("Unknown sortBy")
	}
	slices.SortStableFunc(matching, func(a, b T) int {
		return compareSortKeys(accessor(a), accessor(b), attribute.CaseExact, query.Descending())
	})

	return matching, nil
}

func compareSortKeys(a, b any, caseExact, descending bool) int {
	aMissing, bMissing := isMissingSortValue(a), isMissingSortValue(b)
	switch {
	case aMissing && bMissing:
		return 0
	case aMissing:
		return 1
	case bMissing:
		return -1
	}

	result := compareSortValue(a, b, caseExact)
	if descending {
		result = -result
	}
	return result
}

func isMissingSortValue(value any) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return v == ""
	default:
		return false
	}
}

func compareSortValue(a, b any, caseExact bool) int {
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		if !ok {
			return 0
		}
		if !caseExact {
			av, bv = strings.ToLower(av), strings.ToLower(bv)
		}
		return cmp.Compare(av, bv)
	case bool:
		bv, ok := b.(bool)
		if !ok {
			return 0
		}
		return cmp.Compare(boolSortRank(av), boolSortRank(bv))
	case int64:
		bv, ok := b.(int64)
		if !ok {
			return 0
		}
		return cmp.Compare(av, bv)
	case float64:
		bv, ok := b.(float64)
		if !ok {
			return 0
		}
		return cmp.Compare(av, bv)
	case time.Time:
		bv, ok := b.(time.Time)
		if !ok {
			return 0
		}
		return av.Compare(bv)
	default:
		return 0
	}
}

func boolSortRank(v bool) int {
	if v {
		return 1
	}
	return 0
}

func weakETag(t time.Time) string {
	return `W/"` + strconv.FormatInt(t.UnixNano(), 10) + `"`
}
