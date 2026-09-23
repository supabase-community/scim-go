package server

import (
	"context"

	"github.com/supabase-community/scim-go/pkg/protocol"
)

type service[T Entity] struct {
	repo       Repository[T]
	validators []Validator[T]
}

func newService[T Entity](repo Repository[T], validators ...Validator[T]) *service[T] {
	return &service[T]{repo: repo, validators: validators}
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
	return s.repo.Create(ctx, item)
}

func (s *service[T]) Replace(ctx context.Context, item T) (T, error) {
	if err := s.validate(ctx, item); err != nil {
		var zero T
		return zero, err
	}
	return s.repo.Replace(ctx, item)
}

func (s *service[T]) Delete(ctx context.Context, id string, version string) error {
	return s.repo.Delete(ctx, id, version)
}

func (s *service[T]) validate(ctx context.Context, item T) error {
	for _, validate := range s.validators {
		if err := validate(ctx, item); err != nil {
			return err
		}
	}
	return nil
}
