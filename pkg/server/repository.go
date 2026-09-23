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

type memoryRepository[T Entity] struct {
	mu               sync.RWMutex
	once             sync.Once
	resource         *Resource[T]
	endpoint         string
	schema           *core.Schema
	items            []T
	accessors        Accessors[T]
	elementAccessors map[*core.Attribute]func(any) any
	elements         map[*core.Attribute]func(T) []any
	evaluator        protocol.Evaluator[predicate]
}

// NewMemoryRepository stores the resources of resource in memory, for tests and reference servers.
func NewMemoryRepository[T Entity](resource *Resource[T]) Repository[T] {
	return &memoryRepository[T]{resource: resource, items: []T{}}
}

func (r *memoryRepository[T]) ready() {
	r.once.Do(func() {
		fields := r.resource.allFields()
		r.endpoint = r.resource.basePath + r.resource.endpoint
		r.schema = r.resource.schema(r.resource.basePath)
		r.accessors = withCommonAccessors(fields.Accessors())
		r.elementAccessors = fields.ElementAccessors()
		r.elements = fields.Elements()
		r.evaluator = NewVisitor(fields)
	})
}

func (r *memoryRepository[T]) Get(_ context.Context, id string) (T, error) {
	r.ready()
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, item := range r.items {
		if item.ResourceID() == id {
			return copyOf(item), nil
		}
	}
	var zero T
	return zero, scimerrors.ErrNotFound("Not found")
}

func (r *memoryRepository[T]) List(_ context.Context, query *protocol.SearchRequest) ([]T, int, error) {
	r.ready()
	r.mu.RLock()
	defer r.mu.RUnlock()
	matching, err := r.sortBy(query)
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

func (r *memoryRepository[T]) Create(_ context.Context, item T) (T, error) {
	r.ready()
	r.mu.Lock()
	defer r.mu.Unlock()
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

	r.items = append(r.items, item)
	return copyOf(item), nil
}

func (r *memoryRepository[T]) Replace(_ context.Context, item T) (T, error) {
	r.ready()
	r.mu.Lock()
	defer r.mu.Unlock()
	item = copyOf(item)
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
			return copyOf(item), nil
		}
	}
	var zero T
	return zero, scimerrors.ErrNotFound("Not found")
}

func (r *memoryRepository[T]) Delete(_ context.Context, id string, version string) error {
	r.ready()
	r.mu.Lock()
	defer r.mu.Unlock()
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

func (r *memoryRepository[T]) filterBy(query *protocol.SearchRequest) ([]T, error) {
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

func (r *memoryRepository[T]) sortBy(query *protocol.SearchRequest) ([]T, error) {
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
func (r *memoryRepository[T]) sortKey(parent, attribute *core.Attribute) (Accessor[T], bool) {
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
