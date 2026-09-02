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

type UserRepository struct {
	mu      sync.RWMutex
	baseURL string
	items   map[string]*core.User
}

func NewMemoryUserRepository(baseURL string) scim.Repository[*core.User] {
	return &UserRepository{baseURL: baseURL, items: make(map[string]*core.User)}
}

func (r *UserRepository) Get(ctx context.Context, id string) (*core.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, ok := r.items[id]
	if !ok {
		return nil, scim.ErrNotFound
	}
	return item, nil
}

func (r *UserRepository) List(ctx context.Context, query *protocol.SearchRequest) ([]*core.User, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	all := make([]*core.User, 0, len(r.items))
	for _, item := range r.items {
		all = append(all, item)
	}
	slices.SortFunc(all, func(a, b *core.User) int { return strings.Compare(a.ID, b.ID) })

	total := len(all)
	start := min(query.Offset(), total)
	end := min(start+query.Count, total)
	return all[start:end], total, nil
}

func (r *UserRepository) Create(ctx context.Context, user *core.User) (*core.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	user.ID = uuid.Must(uuid.NewV4()).String()
	user.Meta = core.NewMeta(r.baseURL, core.KindUser).For(user)
	r.items[user.ID] = user
	return user, nil
}

func (r *UserRepository) Replace(ctx context.Context, id string, user *core.User) (*core.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.items[id]; !ok {
		return nil, scim.ErrNotFound
	}
	user.ID = id
	user.Meta = core.NewMeta(r.baseURL, core.KindUser).For(user)
	r.items[id] = user
	return user, nil
}

func (r *UserRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.items[id]; !ok {
		return scim.ErrNotFound
	}
	delete(r.items, id)
	return nil
}
