package server

// type Entity struct {
// }

// func (e *Entity) ResourceID() string {
// 	return ""
// }

// type MemoryRepository struct {
// 	mu      sync.RWMutex
// 	baseURL string
// 	items   map[string]Entity
// }

// func NewMemoryRepository(baseURL string) scim.Repository[Entity] {
// 	return &MemoryRepository{
// 		baseURL: baseURL,
// 		items:   make(map[string]Entity),
// 	}
// }

// func (r *MemoryRepository) Get(ctx context.Context, id string) (Entity, error) {
// 	r.mu.RLock()
// 	defer r.mu.RUnlock()

// 	item, ok := r.items[id]
// 	if !ok {
// 		return nil, scim.ErrNotFound
// 	}
// 	return item, nil
// }

// func (r *MemoryRepository) List(ctx context.Context, query *protocol.SearchRequest) ([]Entity, int, error) {
// 	r.mu.RLock()
// 	defer r.mu.RUnlock()

// 	all := make([]Entity, 0, len(r.items))
// 	for _, item := range r.items {
// 		all = append(all, item)
// 	}
// 	slices.SortFunc(all, func(a, b Entity) int { return strings.Compare(a.ID, b.ID) })

// 	total := len(all)
// 	start := min(query.Offset(), total)
// 	end := min(start+query.Count, total)
// 	return all[start:end], total, nil
// }

// func (r *MemoryRepository) Create(ctx context.Context, resource Entity) (Entity, error) {
// 	r.mu.Lock()
// 	defer r.mu.Unlock()

// 	resource.ID = uuid.Must(uuid.NewV4()).String()
// 	resource.Meta = core.NewMeta(r.baseURL, core.KindUser).For(resource)
// 	r.items[resource.ID] = resource
// 	return resource, nil
// }

// func (r *MemoryRepository) Replace(ctx context.Context, id string, resource Entity) (Entity, error) {
// 	r.mu.Lock()
// 	defer r.mu.Unlock()

// 	if _, ok := r.items[id]; !ok {
// 		return nil, scim.ErrNotFound
// 	}
// 	resource.ID = id
// 	resource.Meta = core.NewMeta(r.baseURL, core.KindUser).For(resource)
// 	r.items[id] = resource
// 	return resource, nil
// }

// func (r *MemoryRepository) Delete(ctx context.Context, id string) error {
// 	r.mu.Lock()
// 	defer r.mu.Unlock()

// 	if _, ok := r.items[id]; !ok {
// 		return scim.ErrNotFound
// 	}
// 	delete(r.items, id)
// 	return nil
// }
