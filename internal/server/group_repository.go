package server

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/gofrs/uuid"
	"github.com/supabase-community/go-scim/internal/scim"
	"github.com/supabase-community/go-scim/pkg/core"
	"github.com/supabase-community/go-scim/pkg/protocol"
)

type memoryGroupRepository struct {
	mu      sync.RWMutex
	baseURL string
	items   map[string]*core.Group
}

func NewMemoryGroupRepository(baseURL string) scim.Repository[*core.Group] {
	return &memoryGroupRepository{baseURL: baseURL, items: make(map[string]*core.Group)}
}

func (r *memoryGroupRepository) Get(ctx context.Context, id string) (*core.Group, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, ok := r.items[id]
	if !ok {
		return nil, scim.ErrNotFound
	}
	return item, nil
}

func (r *memoryGroupRepository) List(ctx context.Context, query *protocol.SearchRequest) ([]*core.Group, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	all := make([]*core.Group, 0, len(r.items))
	for _, item := range r.items {
		all = append(all, item)
	}
	slices.SortFunc(all, func(a, b *core.Group) int { return strings.Compare(a.ID, b.ID) })

	total := len(all)
	start := min(query.Offset(), total)
	end := min(start+query.Count, total)
	return all[start:end], total, nil
}

func (r *memoryGroupRepository) Create(ctx context.Context, group *core.Group) (*core.Group, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	group.ID = uuid.Must(uuid.NewV4()).String()
	r.items[group.ID] = group
	return group, nil
}

func (r *memoryGroupRepository) Replace(ctx context.Context, id string, group *core.Group) (*core.Group, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.items[id]; !ok {
		return nil, scim.ErrNotFound
	}
	group.ID = id
	r.items[id] = group
	return group, nil
}

func (r *memoryGroupRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.items[id]; !ok {
		return scim.ErrNotFound
	}
	delete(r.items, id)
	return nil
}
