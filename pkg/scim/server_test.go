package scim

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"mokhan.ca/go/scim/pkg/core"
	"mokhan.ca/go/scim/pkg/protocol"
)

type fakeUserRepo struct {
	mu    sync.Mutex
	items map[string]*core.User
	seq   int
}

func newFakeUserRepo() *fakeUserRepo { return &fakeUserRepo{items: map[string]*core.User{}} }

func (r *fakeUserRepo) Get(ctx context.Context, id string) (*core.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return item, nil
}

func (r *fakeUserRepo) List(ctx context.Context, query *protocol.SearchRequest) ([]*core.User, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var all []*core.User
	for _, item := range r.items {
		if filter := query.ParsedFilter(); filter != nil {
			ok, err := protocol.Matches(filter, item)
			if err != nil || !ok {
				continue
			}
		}
		all = append(all, item)
	}
	return all, len(all), nil
}

func (r *fakeUserRepo) Create(ctx context.Context, user *core.User) (*core.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	user.ID = "user-" + strconv.Itoa(r.seq)
	user.Meta = core.NewMeta("http://example.com/scim/v2", core.KindUser).For(user)
	r.items[user.ID] = user
	return user, nil
}

func (r *fakeUserRepo) Replace(ctx context.Context, id string, user *core.User) (*core.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return nil, ErrNotFound
	}
	user.ID = id
	user.Meta = core.NewMeta("http://example.com/scim/v2", core.KindUser).For(user)
	r.items[id] = user
	return user, nil
}

func (r *fakeUserRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}

type fakeGroupRepo struct {
	mu    sync.Mutex
	items map[string]*core.Group
	seq   int
}

func newFakeGroupRepo() *fakeGroupRepo { return &fakeGroupRepo{items: map[string]*core.Group{}} }

func (r *fakeGroupRepo) Get(ctx context.Context, id string) (*core.Group, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return item, nil
}

func (r *fakeGroupRepo) List(ctx context.Context, query *protocol.SearchRequest) ([]*core.Group, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var all []*core.Group
	for _, item := range r.items {
		all = append(all, item)
	}
	return all, len(all), nil
}

func (r *fakeGroupRepo) Create(ctx context.Context, group *core.Group) (*core.Group, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	group.ID = "group-" + strconv.Itoa(r.seq)
	group.Meta = core.NewMeta("http://example.com/scim/v2", core.KindGroup).For(group)
	r.items[group.ID] = group
	return group, nil
}

func (r *fakeGroupRepo) Replace(ctx context.Context, id string, group *core.Group) (*core.Group, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return nil, ErrNotFound
	}
	group.ID = id
	group.Meta = core.NewMeta("http://example.com/scim/v2", core.KindGroup).For(group)
	r.items[id] = group
	return group, nil
}

func (r *fakeGroupRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}

func newTestServer() (*Server, *fakeUserRepo, *fakeGroupRepo) {
	users := newFakeUserRepo()
	groups := newFakeGroupRepo()
	srv := NewServer("http://example.com", users, groups, nil, nil)
	return srv, users, groups
}

func doRequest(t *testing.T, handler func(http.ResponseWriter, *http.Request) error, method, path, id, body string) *httptest.ResponseRecorder {
	t.Helper()

	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if id != "" {
		r.SetPathValue("id", id)
	}
	w := httptest.NewRecorder()
	require.NoError(t, handler(w, r))
	return w
}

func TestServerUsers(t *testing.T) {
	t.Run("creates a user and locates it by id", func(t *testing.T) {
		srv, _, _ := newTestServer()

		w := doRequest(t, srv.CreateUser, http.MethodPost, "/Users", "", `{
			"schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"],
			"userName": "bjensen"
		}`)

		require.Equal(t, http.StatusCreated, w.Code)
		require.NotEmpty(t, w.Header().Get("Location"))

		var created core.User
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
		require.Equal(t, "bjensen", created.UserName)
		require.NotEmpty(t, created.ID)

		byID := doRequest(t, srv.UserByID, http.MethodGet, "/Users/"+created.ID, created.ID, "")
		require.Equal(t, http.StatusOK, byID.Code)
	})

	t.Run("rejects a user missing userName", func(t *testing.T) {
		srv, _, _ := newTestServer()

		w := doRequest(t, srv.CreateUser, http.MethodPost, "/Users", "", `{
			"schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"]
		}`)

		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("404s for an unknown id", func(t *testing.T) {
		srv, _, _ := newTestServer()

		w := doRequest(t, srv.UserByID, http.MethodGet, "/Users/missing", "missing", "")

		require.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("replaces and deletes a user", func(t *testing.T) {
		srv, _, _ := newTestServer()

		created := doRequest(t, srv.CreateUser, http.MethodPost, "/Users", "", `{
			"schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"],
			"userName": "bjensen"
		}`)
		var user core.User
		require.NoError(t, json.Unmarshal(created.Body.Bytes(), &user))

		replaced := doRequest(t, srv.ReplaceUser, http.MethodPut, "/Users/"+user.ID, user.ID, `{
			"schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"],
			"userName": "bjensen2"
		}`)
		require.Equal(t, http.StatusOK, replaced.Code)

		deleted := doRequest(t, srv.DeleteUser, http.MethodDelete, "/Users/"+user.ID, user.ID, "")
		require.Equal(t, http.StatusNoContent, deleted.Code)

		gone := doRequest(t, srv.UserByID, http.MethodGet, "/Users/"+user.ID, user.ID, "")
		require.Equal(t, http.StatusNotFound, gone.Code)
	})

	t.Run("filters the list through the repository", func(t *testing.T) {
		srv, _, _ := newTestServer()

		doRequest(t, srv.CreateUser, http.MethodPost, "/Users", "", `{
			"schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"], "userName": "alice"
		}`)
		doRequest(t, srv.CreateUser, http.MethodPost, "/Users", "", `{
			"schemas": ["urn:ietf:params:scim:schemas:core:2.0:User"], "userName": "bob"
		}`)

		w := doRequest(t, srv.Users, http.MethodGet, `/Users?filter=userName+eq+"alice"`, "", "")
		require.Equal(t, http.StatusOK, w.Code)

		var list protocol.ListResponse[core.User]
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &list))
		require.Equal(t, 1, list.TotalResults)
		require.Equal(t, "alice", list.Resources[0].UserName)
	})
}

func TestServerGroups(t *testing.T) {
	t.Run("creates a group and locates it by id", func(t *testing.T) {
		srv, _, _ := newTestServer()

		w := doRequest(t, srv.CreateGroup, http.MethodPost, "/Groups", "", `{
			"schemas": ["urn:ietf:params:scim:schemas:core:2.0:Group"],
			"displayName": "Engineers"
		}`)

		require.Equal(t, http.StatusCreated, w.Code)
		require.NotEmpty(t, w.Header().Get("Location"))

		var created core.Group
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))
		require.Equal(t, "Engineers", created.DisplayName)

		byID := doRequest(t, srv.GroupByID, http.MethodGet, "/Groups/"+created.ID, created.ID, "")
		require.Equal(t, http.StatusOK, byID.Code)
	})

	t.Run("rejects a group missing displayName", func(t *testing.T) {
		srv, _, _ := newTestServer()

		w := doRequest(t, srv.CreateGroup, http.MethodPost, "/Groups", "", `{
			"schemas": ["urn:ietf:params:scim:schemas:core:2.0:Group"]
		}`)

		require.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("404s for an unknown id", func(t *testing.T) {
		srv, _, _ := newTestServer()

		w := doRequest(t, srv.GroupByID, http.MethodGet, "/Groups/missing", "missing", "")

		require.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestServerServiceProviderConfig(t *testing.T) {
	srv, _, _ := newTestServer()

	w := doRequest(t, srv.ServiceProviderConfig, http.MethodGet, "/ServiceProviderConfig", "", "")

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Body.String(), `"filter":{"supported":true`)
}
