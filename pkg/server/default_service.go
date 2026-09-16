package server

import (
	"context"
	"time"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// DefaultService wraps a Repository, assigning ID/Meta and running schema
// validation plus any Validator hooks before persisting.
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
	if err := s.validate(ctx, item, ""); err != nil {
		var zero T
		return zero, err
	}
	now := time.Now().UTC()
	item.SetID(randomHex(16))
	item.SetMeta(core.Meta{ResourceType: s.schema.Name, Created: now, LastModified: now, Version: randomHex(8)})
	return s.repo.Create(ctx, item)
}

func (s *DefaultService[T]) Replace(ctx context.Context, id string, item T) (T, error) {
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		var zero T
		return zero, err
	}
	if err := s.validate(ctx, item, id); err != nil {
		var zero T
		return zero, err
	}
	meta := existing.GetMeta()
	meta.LastModified = time.Now().UTC()
	meta.Version = randomHex(8)
	item.SetID(existing.ResourceID())
	item.SetMeta(meta)
	return s.repo.Replace(ctx, id, item)
}

func (s *DefaultService[T]) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

func (s *DefaultService[T]) validate(ctx context.Context, item T, excludeID string) error {
	if err := s.schema.Validate(item); err != nil {
		return scimerrors.ErrInvalidValue(err.Error())
	}
	for _, validate := range s.validators {
		if err := validate(ctx, item, excludeID); err != nil {
			return err
		}
	}
	return nil
}
