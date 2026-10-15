package server

import (
	"context"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

type stamped[T core.Resource] struct {
	Service[T]
	resourceType core.ResourceTypeName
	location     string
}

// stamp sets meta.resourceType and meta.location on every resource next returns, per RFC 7643, Section 3.1.
func stamp[T core.Resource](next Service[T], resourceType core.ResourceTypeName, location string) Service[T] {
	return &stamped[T]{Service: next, resourceType: resourceType, location: location}
}

func (s *stamped[T]) List(ctx context.Context, query *protocol.SearchRequest) ([]T, int, error) {
	items, total, err := s.Service.List(ctx, query)
	for _, item := range items {
		s.apply(item)
	}
	return items, total, err
}

func (s *stamped[T]) Get(ctx context.Context, id string) (T, error) {
	return s.applied(s.Service.Get(ctx, id))
}

func (s *stamped[T]) Create(ctx context.Context, document core.Object) (T, error) {
	return s.applied(s.Service.Create(ctx, document))
}

func (s *stamped[T]) Replace(ctx context.Context, req *protocol.ReplaceRequest) (T, error) {
	return s.applied(s.Service.Replace(ctx, req))
}

func (s *stamped[T]) Patch(ctx context.Context, req *protocol.PatchRequest) (T, error) {
	return s.applied(s.Service.Patch(ctx, req))
}

func (s *stamped[T]) applied(item T, err error) (T, error) {
	if err == nil {
		s.apply(item)
	}
	return item, err
}

func (s *stamped[T]) apply(item T) {
	common := item.Common()
	common.Meta.ResourceType = s.resourceType
	common.Meta.Location = s.location + "/" + common.ID
}
