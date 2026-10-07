package server

import (
	"context"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

const (
	OpCreate  Operation = "create"
	OpReplace Operation = "replace"
	OpPatch   Operation = "patch"
	OpDelete  Operation = "delete"
)

type Operation string

type Event[T core.Resource] struct {
	Op       Operation
	ID       string
	Resource T
}

type Hooks[T core.Resource] struct {
	Before func(ctx context.Context, event Event[T]) error
	After  func(ctx context.Context, event Event[T]) error
}

type hooked[T core.Resource] struct {
	Service[T]
	hooks Hooks[T]
}

func (h Hooks[T]) Wrap(next Service[T]) Service[T] {
	return &hooked[T]{Service: next, hooks: h}
}

func (h *hooked[T]) Create(ctx context.Context, document core.Object) (T, error) {
	return h.around(ctx, Event[T]{Op: OpCreate}, func() (T, error) {
		return h.Service.Create(ctx, document)
	})
}

func (h *hooked[T]) Replace(ctx context.Context, req *protocol.ReplaceRequest) (T, error) {
	return h.around(ctx, Event[T]{Op: OpReplace, ID: req.ID}, func() (T, error) {
		return h.Service.Replace(ctx, req)
	})
}

func (h *hooked[T]) Patch(ctx context.Context, req *protocol.PatchRequest) (T, error) {
	return h.around(ctx, Event[T]{Op: OpPatch, ID: req.ID}, func() (T, error) {
		return h.Service.Patch(ctx, req)
	})
}

func (h *hooked[T]) Delete(ctx context.Context, req *protocol.DeleteRequest) error {
	event := Event[T]{Op: OpDelete, ID: req.ID}
	if err := publish(ctx, h.hooks.Before, event); err != nil {
		return err
	}
	if err := h.Service.Delete(ctx, req); err != nil {
		return err
	}
	return publish(ctx, h.hooks.After, event)
}

func (h *hooked[T]) around(ctx context.Context, event Event[T], call func() (T, error)) (T, error) {
	var zero T
	if err := publish(ctx, h.hooks.Before, event); err != nil {
		return zero, err
	}
	resource, err := call()
	if err != nil {
		return zero, err
	}
	event.ID, event.Resource = resource.Common().ID, resource
	return resource, publish(ctx, h.hooks.After, event)
}

func publish[T core.Resource](ctx context.Context, hook func(context.Context, Event[T]) error, event Event[T]) error {
	if hook == nil {
		return nil
	}
	return hook(ctx, event)
}
