package server

import (
	"context"
	"strconv"
	"time"
	"uuid"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

type Service[T core.Resource] interface {
	Get(ctx context.Context, id string) (T, error)
	List(ctx context.Context, query *protocol.SearchRequest) (items []T, total int, err error)
	Create(ctx context.Context, item T) (T, error)
	Replace(ctx context.Context, id string, item T) (T, error)
	Delete(ctx context.Context, id string) error
}

type service[T core.Identifiable] struct {
	repo       Repository[T]
	schema     *core.Schema
	validators []Validator[T]
}

func NewService[T core.Identifiable](repo Repository[T], schema *core.Schema, validators ...Validator[T]) Service[T] {
	return &service[T]{repo: repo, schema: schema, validators: validators}
}

func (s *service[T]) Get(ctx context.Context, id string) (T, error) {
	return s.repo.Get(ctx, id)
}

func (s *service[T]) List(ctx context.Context, query *protocol.SearchRequest) ([]T, int, error) {
	return s.repo.List(ctx, query)
}

func (s *service[T]) Create(ctx context.Context, item T) (T, error) {
	if err := s.validate(ctx, item); err != nil {
		var zero T
		return zero, err
	}
	now := time.Now().UTC()
	item.SetID(uuid.NewV7().String())
	item.SetMeta(core.Meta{
		ResourceType: s.schema.Name,
		Created:      now,
		LastModified: now,
		Version:      strconv.FormatInt(now.Unix(), 10),
	})
	return s.repo.Create(ctx, item)
}

func (s *service[T]) Replace(ctx context.Context, id string, item T) (T, error) {
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		var zero T
		return zero, err
	}
	if err := s.validate(ctx, item); err != nil {
		var zero T
		return zero, err
	}
	meta := existing.GetMeta()
	now := time.Now().UTC()
	meta.LastModified = now
	meta.Version = strconv.FormatInt(now.Unix(), 10)
	item.SetID(existing.ResourceID())
	item.SetMeta(meta)
	return s.repo.Replace(ctx, id, item)
}

func (s *service[T]) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *service[T]) validate(ctx context.Context, item T) error {
	for _, validate := range s.validators {
		if err := validate(ctx, item); err != nil {
			return err
		}
	}
	return nil
}
