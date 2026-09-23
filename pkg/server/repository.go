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

// Repository stores resources, per RFC 7643 3.1 / RFC 7644 3.14 / RFC 7643 7: Create/Replace stamp id+meta and atomically return scimerrors.ErrUniqueness on a value collision; Replace/Delete honour a non-empty expected version.
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
	schemas  []*core.Schema
	items    []T
	readers[T]
	evaluator protocol.Evaluator[predicate]
}

// NewRepository stores resources in memory, for tests and reference servers.
func NewRepository[T Entity](endpoint string, schemas []*core.Schema, fields Fields[T]) Repository[T] {
	readers := fields.readers()
	return &repository[T]{
		endpoint:  endpoint,
		schemas:   schemas,
		items:     []T{},
		readers:   readers,
		evaluator: newVisitor[T](readers),
	}
}

func schemaURIs(schemas []*core.Schema) []core.SchemaURI {
	ids := make([]core.SchemaURI, len(schemas))
	for i, schema := range schemas {
		ids[i] = schema.ID
	}
	return ids
}

func (r *repository[T]) Get(_ context.Context, id string) (item T, err error) {
	err = r.withLock(func() error {
		i, err := r.locate(id, "")
		if err != nil {
			return err
		}
		item = r.items[i]
		return nil
	})
	return item, err
}

func (r *repository[T]) List(_ context.Context, query *protocol.SearchRequest) ([]T, int, error) {
	var matching []T
	err := r.withLock(func() (err error) {
		matching, err = r.sortBy(query)
		return err
	})
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
	item.SetSchemas(schemaURIs(r.schemas))
	item.SetMeta(core.Meta{
		ResourceType: r.schemas[0].Name,
		Created:      now,
		LastModified: now,
		Location:     r.endpoint + "/" + id,
		Version:      weakETag(now),
	})

	err := r.withLock(func() error {
		if attribute := r.conflictingAttribute(item); attribute != nil {
			return scimerrors.ErrUniqueness(strconv.Quote(attribute.Name) + " must be unique")
		}
		r.items = append(r.items, item)
		return nil
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return item, nil
}

func (r *repository[T]) Replace(_ context.Context, item T) (T, error) {
	err := r.withLock(func() error {
		i, err := r.locate(item.ResourceID(), item.GetMeta().Version)
		if err != nil {
			return err
		}
		if attribute := r.conflictingAttribute(item); attribute != nil {
			return scimerrors.ErrUniqueness(strconv.Quote(attribute.Name) + " must be unique")
		}
		meta := r.items[i].GetMeta()
		now := time.Now().UTC()
		meta.LastModified = now
		meta.Version = weakETag(now)
		item.SetMeta(meta)
		item.SetSchemas(schemaURIs(r.schemas))
		r.items[i] = item
		return nil
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return item, nil
}

func (r *repository[T]) Delete(_ context.Context, id, version string) error {
	return r.withLock(func() error {
		i, err := r.locate(id, version)
		if err != nil {
			return err
		}
		r.items = slices.Delete(r.items, i, i+1)
		return nil
	})
}

func (r *repository[T]) withLock(fn func() error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return fn()
}

// conflictingAttribute finds a unique attribute of candidate already held by another stored item, per RFC 7643, Section 7.
func (r *repository[T]) conflictingAttribute(candidate T) *core.Attribute {
	for _, other := range r.items {
		if other.ResourceID() == candidate.ResourceID() {
			continue
		}
		for attribute, accessor := range r.accessors {
			if attribute.Uniqueness == core.UniquenessNone {
				continue
			}
			if sharesValue(accessor(other), accessor(candidate), attribute.CaseExact) {
				return attribute
			}
		}
	}
	return nil
}

// sharesValue reports whether a and b hold a common value, per RFC 7644 Section 3.4.2.2 (multi-valued "any match").
func sharesValue(a, b any, caseExact bool) bool {
	for _, x := range valuesOf(a) {
		if isEmpty(x) {
			continue
		}
		for _, y := range valuesOf(b) {
			if sameValue(x, y, caseExact) {
				return true
			}
		}
	}
	return false
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

	predicate, err := protocol.Filter(r.schemas, query.Filter, r.evaluator)
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
