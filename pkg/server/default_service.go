package server

import (
	"context"
	"strconv"
	"time"
	"uuid"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

type DefaultService[T core.Identifiable] struct {
	repo       Repository[T]
	schema     *core.Schema
	validators []Validator[T]
}

func NewDefaultService[T core.Identifiable](repo Repository[T], schema *core.Schema, validators ...Validator[T]) *DefaultService[T] {
	return &DefaultService[T]{repo: repo, schema: schema, validators: validators}
}

func (s *DefaultService[T]) Get(ctx context.Context, id string) (T, error) {
	return s.repo.Get(ctx, id)
}

func (s *DefaultService[T]) List(ctx context.Context, query *protocol.SearchRequest) ([]T, int, error) {
	return s.repo.List(ctx, query)
}

func (s *DefaultService[T]) Create(ctx context.Context, item T) (T, error) {
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

func (s *DefaultService[T]) Replace(ctx context.Context, id string, item T) (T, error) {
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

func (s *DefaultService[T]) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *DefaultService[T]) validate(ctx context.Context, item T) error {
	for _, validate := range s.validators {
		if err := validate(ctx, item); err != nil {
			return err
		}
	}
	return nil
}
