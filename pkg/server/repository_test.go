package server_test

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
	"github.com/supabase-community/scim-go/pkg/server"
	"github.com/supabase-community/scim-go/pkg/store"
)

const memberID = "0198f3a2-7c1e-7b1a-8d3c-2f5a6b7c8d9e"

type fakeStore struct {
	record  store.Record
	err     error
	findErr error
	doc     store.Document
}

func (f *fakeStore) Find(context.Context, string, []store.Reference) (store.Record, error) {
	return f.record, f.findErr
}

func (f *fakeStore) List(context.Context, *protocol.SearchRequest, []store.Reference) ([]store.Record, int, error) {
	return []store.Record{f.record}, 1, f.err
}

func (f *fakeStore) Create(_ context.Context, doc store.Document) (store.Record, error) {
	f.doc = doc
	return f.record, f.err
}

func (f *fakeStore) Update(_ context.Context, doc store.Document, _ string) (store.Record, error) {
	f.doc = doc
	return f.record, f.err
}

func (f *fakeStore) Delete(context.Context, string, string) error {
	return f.err
}

// RFC 7643 Section 7: a "server" unique value SHOULD be unique within the context of the current SCIM endpoint.
func TestRepositoryUniqueness(t *testing.T) {
	noon := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name          string
		first, second *widget
	}{
		{"one instant in two time zones", &widget{When: noon}, &widget{When: noon.In(time.FixedZone("EST", -5*60*60))}},
		{"an element of a complex multi-valued attribute", &widget{Parts: []part{{Serial: "a"}, {Serial: "b"}}}, &widget{Parts: []part{{Serial: "b"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository := server.NewMemoryRepository[*widget]("/Widgets", core.Schemas{core.NewSchema(widgetSchema).WithName("Widget").With(
				core.NewAttribute("when", core.TypeDateTime).UniqueOn(core.UniquenessServer),
				core.NewAttribute("parts", core.TypeComplex).AsMultiValued().UniqueOn(core.UniquenessServer).With(
					core.NewAttribute("serial", core.TypeString),
				),
			)})
			_, err := repository.Create(context.Background(), tc.first)
			require.NoError(t, err)

			_, err = repository.Create(context.Background(), tc.second)

			assert.ErrorIs(t, err, scimerrors.ErrUniqueness(""))
		})
	}
}

func TestRepositoryListBounds(t *testing.T) {
	cases := []struct {
		name  string
		query *protocol.SearchRequest
		want  int
	}{
		{"nil query", nil, 0},
		{"negative count", &protocol.SearchRequest{StartIndex: 1, Count: -1}, 0},
		{"count past the end", &protocol.SearchRequest{StartIndex: 2, Count: math.MaxInt}, 1},
		{"start past the end", &protocol.SearchRequest{StartIndex: 10, Count: 5}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repository := server.NewMemoryRepository[*core.User](basePath+"/Users", userSchemas())
			for _, name := range []string{"a", "b"} {
				_, err := repository.Create(context.Background(), &core.User{UserName: name})
				require.NoError(t, err)
			}

			items, total, err := repository.List(context.Background(), tc.query)

			require.NoError(t, err)
			assert.Equal(t, 2, total)
			assert.Len(t, items, tc.want)
		})
	}
}

// RFC 7644 Section 3.14: a write must not leave the version an earlier write had.
func TestRepositoryUpdateAlwaysChangesTheVersion(t *testing.T) {
	repository := server.NewMemoryRepository[*core.User]("/Users", core.Schemas{core.NewSchema(core.SchemaUser).With(core.UserAttributes()...)})
	user, err := repository.Create(t.Context(), &core.User{UserName: "bjensen"})
	require.NoError(t, err)
	seen := map[string]bool{user.Meta.Version: true}

	for range 50 {
		user, err = repository.Update(t.Context(), user)

		require.NoError(t, err)
		assert.False(t, seen[user.Meta.Version], user.Meta.Version)
		seen[user.Meta.Version] = true
		modified, ok := server.ParseWeakETag(user.Meta.Version)
		require.True(t, ok)
		assert.True(t, modified.Equal(user.Meta.LastModified))
	}
}

func TestRepository(t *testing.T) {
	ctx := context.Background()
	record := store.Record{ID: "1", Body: json.RawMessage(`{"displayName":"Admins"}`)}

	t.Run("creates with a generated id and splits out the links", func(t *testing.T) {
		store := &fakeStore{record: record}
		group := &core.Group{Members: []core.Member{{Value: memberID}}}

		created, err := newGroupRepository(store).Create(ctx, group)

		require.NoError(t, err)
		require.Equal(t, "Admins", created.DisplayName)
		require.NotEmpty(t, store.doc.ID)
		require.Equal(t, []string{memberID}, store.doc.Links[0].Targets)
		require.NotContains(t, string(store.doc.Body), "members")
	})

	t.Run("reads, lists and updates", func(t *testing.T) {
		repository := newGroupRepository(&fakeStore{record: record})

		read, err := repository.Read(ctx, "1")
		require.NoError(t, err)
		require.Equal(t, "1", read.ID)

		items, total, err := repository.List(ctx, nil)
		require.NoError(t, err)
		require.Equal(t, 1, total)
		require.Len(t, items, 1)

		_, err = repository.Update(ctx, read)
		require.NoError(t, err)
	})

	t.Run("passes store errors through", func(t *testing.T) {
		repository := newGroupRepository(&fakeStore{err: scimerrors.ErrUniqueness("taken"), findErr: scimerrors.ErrNotFound("gone")})

		_, _, err := repository.List(ctx, nil)
		require.Error(t, err)
		_, err = repository.Create(ctx, &core.Group{})
		require.Error(t, err)
		_, err = repository.Read(ctx, "1")
		require.Error(t, err)
		_, err = repository.Create(ctx, &core.Group{Members: []core.Member{{Value: "x"}}})
		require.Error(t, err)
	})

	t.Run("answers 412 when the resource exists but the version is stale", func(t *testing.T) {
		repository := newGroupRepository(&fakeStore{err: store.ErrNoMatch})

		_, err := repository.Update(ctx, &core.Group{})
		require.ErrorIs(t, err, scimerrors.ErrPreconditionFailed("resource has changed on the server"))
		require.Error(t, repository.Delete(ctx, &core.Group{}))
	})

	t.Run("answers 404 when the resource is gone", func(t *testing.T) {
		repository := newGroupRepository(&fakeStore{err: store.ErrNoMatch, findErr: scimerrors.ErrNotFound("gone")})

		require.ErrorIs(t, repository.Delete(ctx, &core.Group{}), scimerrors.ErrNotFound("gone"))
	})

	t.Run("deletes", func(t *testing.T) {
		require.NoError(t, newGroupRepository(&fakeStore{}).Delete(ctx, &core.Group{}))
	})
}

func newGroupRepository(store store.Store) server.Repository[*core.Group] {
	return server.NewRepository[*core.Group](store, groupSchemas())
}
