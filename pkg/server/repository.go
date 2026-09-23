package server

import (
	"cmp"
	"context"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
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
	mu               sync.Mutex
	endpoint         string
	schema           *core.Schema
	items            []T
	accessors        accessorSet[T]
	elementAccessors map[*core.Attribute]func(any) any
	elements         map[*core.Attribute]func(T) []any
	evaluator        protocol.Evaluator[predicate]
}

// NewRepository stores resources in memory, for tests and reference servers.
func NewRepository[T Entity](endpoint string, schema *core.Schema, fields Fields[T]) Repository[T] {
	return &repository[T]{
		endpoint:         endpoint,
		schema:           schema,
		items:            []T{},
		accessors:        withCommonAccessors(fields.accessors()),
		elementAccessors: fields.elementAccessors(),
		elements:         fields.elements(),
		evaluator:        newVisitor(fields),
	}
}

func (r *repository[T]) Get(_ context.Context, id string) (item T, err error) {
	r.withLock(func() {
		var i int
		if i, err = r.locate(id, ""); err == nil {
			item = r.items[i]
		}
	})
	return copyOf(item), err
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
	page := make([]T, 0, end-start)
	for _, item := range matching[start:end] {
		page = append(page, copyOf(item))
	}
	return page, total, nil
}

func (r *repository[T]) Create(_ context.Context, item T) (T, error) {
	item = copyOf(item)
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
	return copyOf(item), nil
}

func (r *repository[T]) Replace(_ context.Context, item T) (T, error) {
	item = copyOf(item)
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
	return copyOf(item), nil
}

func (r *repository[T]) Delete(_ context.Context, id string, version string) (err error) {
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

	parent, ok := r.schema.Resolve(path.Name)
	if !ok {
		return []T{}, scimerrors.ErrInvalidValue("Unknown sortBy")
	}

	attribute := parent
	if path.SubAttribute != "" {
		attribute = parent.SubAttribute(path.SubAttribute)
	}

	key, ok := r.sortKey(parent, attribute)
	if !ok {
		return []T{}, scimerrors.ErrInvalidValue("Unknown sortBy")
	}
	slices.SortStableFunc(matching, func(a, b T) int {
		return compareSortKeys(key(a), key(b), attribute.CaseExact, query.Descending())
	})

	return matching, nil
}

// RFC 7644 Section 3.4.2.3: a multi-valued attribute sorts by its primary value, or else its first value.
func (r *repository[T]) sortKey(parent, attribute *core.Attribute) (Accessor[T], bool) {
	elements, multiValued := r.elements[parent]
	read, readable := r.elementAccessors[attribute]
	if !multiValued || !readable {
		accessor, ok := r.accessors[attribute]
		return accessor, ok
	}
	isPrimary := r.elementAccessors[parent.SubAttribute("primary")]
	return func(item T) any {
		element, ok := primaryOrFirst(elements(item), isPrimary)
		if !ok {
			return nil
		}
		return read(element)
	}, true
}

func primaryOrFirst(elements []any, isPrimary func(any) any) (any, bool) {
	if isPrimary != nil {
		for _, element := range elements {
			if isPrimary(element) == true {
				return element, true
			}
		}
	}
	if len(elements) == 0 {
		return nil, false
	}
	return elements[0], true
}

// RFC 7644 Section 3.4.2.3: resources without a value are ordered last if ascending and first if descending.
func compareSortKeys(a, b any, caseExact, descending bool) int {
	result := compareAscending(a, b, caseExact)
	if descending {
		return -result
	}
	return result
}

func compareAscending(a, b any, caseExact bool) int {
	aMissing, bMissing := isMissingSortValue(a), isMissingSortValue(b)
	switch {
	case aMissing && bMissing:
		return 0
	case aMissing:
		return 1
	case bMissing:
		return -1
	}
	return compareSortValue(a, b, caseExact)
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

func copyOf[T any](item T) T {
	value := reflect.ValueOf(item)
	if value.Kind() != reflect.Pointer || value.IsNil() {
		return item
	}
	clone := reflect.New(value.Elem().Type())
	clone.Elem().Set(value.Elem())
	return clone.Interface().(T)
}

func weakETag(t time.Time) string {
	return `W/"` + strconv.FormatInt(t.UnixNano(), 10) + `"`
}
