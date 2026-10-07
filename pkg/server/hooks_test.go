package server_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/server"
)

type recorder struct {
	before, after []server.Event[*core.Group]
}

func (r *recorder) hooks() server.Hooks[*core.Group] {
	return server.Hooks[*core.Group]{
		Before: func(_ context.Context, e server.Event[*core.Group]) error {
			r.before = append(r.before, e)
			return nil
		},
		After: func(_ context.Context, e server.Event[*core.Group]) error {
			r.after = append(r.after, e)
			return nil
		},
	}
}

func TestHooks(t *testing.T) {
	ctx := context.Background()
	newService := func(hooks server.Hooks[*core.Group]) server.Service[*core.Group] {
		repo := server.NewRepository[*core.Group](basePath+"/Groups", groupSchemas())
		return hooks.Wrap(server.NewService(repo, groupSchemas(), server.Limits{}))
	}
	group := func(name string) core.Object {
		return core.Object{"schemas": []any{string(core.SchemaGroup)}, "displayName": name}
	}

	t.Run("reports every write", func(t *testing.T) {
		events := &recorder{}
		service := newService(events.hooks())

		created, err := service.Create(ctx, group("a"))
		require.NoError(t, err)
		id := created.ID
		replaced, err := service.Replace(ctx, &protocol.ReplaceRequest{ID: id, Attributes: group("b")})
		require.NoError(t, err)
		req, err := protocol.DecodePatchRequest(strings.NewReader(`{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[{"op":"replace","path":"displayName","value":"c"}]}`))
		require.NoError(t, err)
		req.ID = id
		patched, err := service.Patch(ctx, req)
		require.NoError(t, err)
		require.NoError(t, service.Delete(ctx, &protocol.DeleteRequest{ID: id}))

		assert.Equal(t, []server.Event[*core.Group]{
			{Op: server.OpCreate},
			{Op: server.OpReplace, ID: id},
			{Op: server.OpPatch, ID: id},
			{Op: server.OpDelete, ID: id},
		}, events.before)
		require.Len(t, events.after, 4)
		assert.Equal(t, server.Event[*core.Group]{Op: server.OpCreate, ID: id, Resource: created}, events.after[0])
		assert.Equal(t, "c", patched.DisplayName)
		assert.Equal(t, server.Event[*core.Group]{Op: server.OpReplace, ID: id, Resource: replaced}, events.after[1])
		assert.Equal(t, server.Event[*core.Group]{Op: server.OpPatch, ID: id, Resource: patched}, events.after[2])
		assert.Equal(t, server.Event[*core.Group]{Op: server.OpDelete, ID: id}, events.after[3])
	})

	t.Run("a Before error stops the write", func(t *testing.T) {
		rejected := errors.New("rejected")
		var after []server.Event[*core.Group]
		service := newService(server.Hooks[*core.Group]{
			Before: func(context.Context, server.Event[*core.Group]) error { return rejected },
			After: func(_ context.Context, e server.Event[*core.Group]) error {
				after = append(after, e)
				return nil
			},
		})

		_, err := service.Create(ctx, group("a"))

		require.ErrorIs(t, err, rejected)
		assert.Empty(t, after)
		_, total, err := service.List(ctx, &protocol.SearchRequest{StartIndex: 1, Count: 10})
		require.NoError(t, err)
		assert.Zero(t, total)
	})

	t.Run("an After error reaches the caller", func(t *testing.T) {
		failed := errors.New("failed")
		service := newService(server.Hooks[*core.Group]{
			After: func(context.Context, server.Event[*core.Group]) error { return failed },
		})

		_, err := service.Create(ctx, group("a"))

		assert.ErrorIs(t, err, failed)
	})

	t.Run("nil hooks change nothing", func(t *testing.T) {
		service := newService(server.Hooks[*core.Group]{})

		created, err := service.Create(ctx, group("a"))

		require.NoError(t, err)
		assert.Equal(t, "a", created.DisplayName)
	})
}
