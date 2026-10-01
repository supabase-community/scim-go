package server

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strconv"
	"sync"
	"time"
	"uuid"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// Repository stores resources and enforces uniqueness atomically, per RFC 7643 Section 7.
type Repository[T core.Resource] interface {
	List(ctx context.Context, query *protocol.SearchRequest) (items []T, total int, err error)
	Get(ctx context.Context, id string) (T, error)
	Create(ctx context.Context, item T) (T, error)
	Update(ctx context.Context, id, version string, change func(current T) (T, error)) (T, error)
	Delete(ctx context.Context, id, version string) error
}

// AttributePatcher applies an add/remove delta to one attribute without the full resource; MaxResourceBytes is not enforced on this path.
type AttributePatcher[T core.Resource] interface {
	PatchAttribute(ctx context.Context, id, version string, delta AttributeDelta) (meta core.Meta, changed bool, err error)
}

type AttributeDelta struct {
	Attribute string
	Added     []core.Object
	Removed   []string
}

type repository[T core.Resource] struct {
	mu       sync.Mutex
	endpoint string
	schemas  core.Schemas
	rows     []row[T]
	index    map[string]int
	owners   owners
	fields
	evaluator protocol.Evaluator[predicate]
}

type row[T core.Resource] struct {
	item   T
	object core.Object
}

// NewRepository stores resources in memory, for tests and reference servers.
func NewRepository[T core.Resource](endpoint string, schemas core.Schemas) Repository[T] {
	fields := newFields(schemas)
	return &repository[T]{
		endpoint:  endpoint,
		schemas:   schemas,
		rows:      []row[T]{},
		index:     map[string]int{},
		owners:    newOwners(fields),
		fields:    fields,
		evaluator: newEvaluator(fields),
	}
}

func (r *repository[T]) Get(_ context.Context, id string) (item T, err error) {
	err = r.withLock(func() error {
		i, err := r.locate(id, "")
		if err != nil {
			return err
		}
		item = r.rows[i].item
		return nil
	})
	return item, err
}

func (r *repository[T]) List(_ context.Context, query *protocol.SearchRequest) ([]T, int, error) {
	items, total := []T{}, 0
	err := r.withLock(func() error {
		matching, err := r.sortBy(query)
		if err != nil {
			return err
		}
		total = len(matching)
		start := min(query.Offset(), total)
		for _, row := range matching[start:min(start+query.Count, total)] {
			items = append(items, row.item)
		}
		return nil
	})
	if err != nil {
		return []T{}, 0, err
	}
	return items, total, nil
}

func (r *repository[T]) Create(_ context.Context, item T) (T, error) {
	now := time.Now().UTC()
	common := item.Common()
	common.ID = uuid.NewV7().String()
	common.Meta = core.Meta{
		ResourceType: r.schemas[0].Name,
		Created:      now,
		LastModified: now,
		Location:     r.endpoint + "/" + common.ID,
		Version:      weakETag(now),
	}

	err := r.withLock(func() error {
		created, err := r.rowOf(item)
		if err != nil {
			return err
		}
		r.index[common.ID] = len(r.rows)
		r.rows = append(r.rows, created)
		r.owners.claim(common.ID, created.object)
		return nil
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return item, nil
}

func (r *repository[T]) Update(_ context.Context, id, version string, change func(current T) (T, error)) (item T, err error) {
	err = r.withLock(func() error {
		i, err := r.locate(id, version)
		if err != nil {
			return err
		}
		if item, err = change(r.rows[i].item); err != nil {
			return err
		}
		return r.store(i, item)
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return item, nil
}

func (r *repository[T]) PatchAttribute(_ context.Context, id, version string, delta AttributeDelta) (meta core.Meta, changed bool, err error) {
	err = r.withLock(func() error {
		i, err := r.locate(id, version)
		if err != nil {
			return err
		}
		existing := r.rows[i]
		meta = existing.item.Common().Meta
		attr, ok := r.schemas.Resolve("", delta.Attribute, "")
		if !ok {
			return scimerrors.ErrInvalidPath(strconv.Quote(delta.Attribute) + " is unknown")
		}
		elements, _ := existing.object.Get(delta.Attribute).([]any)
		next, didChange := applyMemberDelta(attr, elements, delta.Added, delta.Removed)
		if !didChange {
			return nil
		}
		replaced, newMeta, err := r.replaceAttribute(existing, delta.Attribute, next)
		if err != nil {
			return err
		}
		r.owners.release(id, existing.object)
		r.owners.claim(id, replaced.object)
		r.rows[i] = replaced
		meta, changed = newMeta, true
		return nil
	})
	return meta, changed, err
}

func (r *repository[T]) Delete(_ context.Context, id, version string) error {
	return r.withLock(func() error {
		i, err := r.locate(id, version)
		if err != nil {
			return err
		}
		r.owners.release(id, r.rows[i].object)
		r.rows = slices.Delete(r.rows, i, i+1)
		delete(r.index, id)
		for j := i; j < len(r.rows); j++ {
			r.index[r.rows[j].item.Common().ID] = j
		}
		return nil
	})
}

func (r *repository[T]) store(i int, item T) error {
	common := item.Common()
	common.ID = r.rows[i].item.Common().ID
	common.Meta = bumpMeta(r.rows[i].item.Common().Meta)
	replaced, err := r.rowOf(item)
	if err != nil {
		return err
	}
	r.owners.release(common.ID, r.rows[i].object)
	r.owners.claim(common.ID, replaced.object)
	r.rows[i] = replaced
	return nil
}

func (r *repository[T]) withLock(fn func() error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return fn()
}

// RFC 7644 Section 3.14: a non-empty version must match the stored one.
func (r *repository[T]) locate(id, version string) (int, error) {
	i, ok := r.index[id]
	switch {
	case !ok:
		return i, scimerrors.ErrNotFound("Not found")
	case version != "" && r.rows[i].item.Common().Meta.Version != version:
		return i, scimerrors.ErrPreconditionFailed("resource has changed on the server")
	}
	return i, nil
}

// rowOf pairs item with its object and rejects a value collision, per RFC 7643, Section 7.
func (r *repository[T]) rowOf(item T) (row[T], error) {
	candidate, err := core.NewObject(item)
	if err != nil {
		return row[T]{}, err
	}
	if attribute := r.owners.conflict(item.Common().ID, candidate); attribute != nil {
		return row[T]{}, scimerrors.ErrUniqueness(strconv.Quote(attribute.Name) + " must be unique")
	}
	return row[T]{item: item, object: candidate}, nil
}

func (r *repository[T]) filterBy(query *protocol.SearchRequest) ([]row[T], error) {
	if query.Filter == "" {
		return r.rows, nil
	}

	predicate, err := protocol.Filter(r.schemas, query.Filter, r.evaluator)
	if err != nil {
		return nil, err
	}
	matching := []row[T]{}
	for _, row := range r.rows {
		if predicate(row.object) {
			matching = append(matching, row)
		}
	}
	return matching, nil
}

func (r *repository[T]) sortBy(query *protocol.SearchRequest) ([]row[T], error) {
	matching, err := r.filterBy(query)
	if err != nil {
		return nil, err
	}
	if query.SortBy == "" {
		return matching, nil
	}

	parent, attribute, err := query.SortAttribute(r.schemas)
	if err != nil {
		return nil, err
	}

	key, ok := r.sortKey(parent, attribute)
	if !ok {
		return nil, scimerrors.ErrInvalidValue("Unknown sortBy")
	}
	matching = slices.Clone(matching)
	slices.SortStableFunc(matching, func(a, b row[T]) int {
		return compareSortKeys(attribute, key(a.object), key(b.object), query.Descending())
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
		return coerce(attribute, asObject(element).Get(attribute.Name))
	}, true
}

func weakETag(t time.Time) string {
	return `W/"` + strconv.FormatInt(t.UnixNano(), 10) + `"`
}

func (r *repository[T]) replaceAttribute(existing row[T], attribute string, next []any) (row[T], core.Meta, error) {
	object := maps.Clone(existing.object)
	object.Set(attribute, next)
	var item T
	raw, err := json.Marshal(object)
	if err != nil {
		return row[T]{}, core.Meta{}, err
	}
	if err := json.Unmarshal(raw, &item); err != nil {
		return row[T]{}, core.Meta{}, err
	}
	common := item.Common()
	common.Meta = bumpMeta(existing.item.Common().Meta)
	replaced, err := r.rowOf(item)
	if err != nil {
		return row[T]{}, core.Meta{}, err
	}
	return replaced, common.Meta, nil
}

func bumpMeta(current core.Meta) core.Meta {
	now := time.Now().UTC()
	current.LastModified = now
	current.Version = weakETag(now)
	return current
}

// applyMemberDelta drops removed identities and appends added ones not already present, per RFC 7644 Section 3.5.2.1.
func applyMemberDelta(attribute *core.Attribute, elements []any, added []core.Object, removed []string) ([]any, bool) {
	next := slices.Clone(elements)
	changed := false
	for _, literal := range removed {
		key := value.Identity(attribute, core.Object{"value": literal})
		before := len(next)
		next = slices.DeleteFunc(next, func(element any) bool {
			return value.Identity(attribute, asObject(element)) == key
		})
		changed = changed || len(next) != before
	}
	present := make(map[string]bool, len(next))
	for _, element := range next {
		present[value.Identity(attribute, asObject(element))] = true
	}
	for _, object := range added {
		key := value.Identity(attribute, object)
		if present[key] {
			continue
		}
		present[key] = true
		next = append(next, map[string]any(object))
		changed = true
	}
	return next, changed
}
