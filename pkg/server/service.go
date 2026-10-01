package server

import (
	"context"
	"encoding/json"
	"maps"
	"reflect"
	"strconv"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type Service[T core.Resource] interface {
	List(ctx context.Context, query *protocol.SearchRequest) (items []T, total int, err error)
	Get(ctx context.Context, id string) (T, error)
	Create(ctx context.Context, item T) (T, error)
	Replace(ctx context.Context, item T) (T, error)
	Delete(ctx context.Context, id, version string) error
}

type service[T core.Resource] struct {
	repo       Repository[T]
	validators []Validator[T]
}

func NewService[T core.Resource](repo Repository[T], validators ...Validator[T]) Service[T] {
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

func (s *service[T]) Delete(ctx context.Context, id, version string) error {
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

// stampSchemas lists the base schema plus every extension with assigned data, per RFC 7643, Section 3.
func stampSchemas[T core.Resource](schemas core.Schemas, resource T) (core.Object, error) {
	object, err := core.NewObject(resource)
	if err != nil {
		return nil, err
	}
	uris := []core.SchemaURI{schemas.Base().ID}
	for _, extension := range schemas.Extensions() {
		if !value.IsUnassigned(object.Get(string(extension.ID))) {
			uris = append(uris, extension.ID)
		}
	}
	resource.Common().Schemas = uris
	object.Set("schemas", schemaURIsToAny(uris))
	return object, nil
}

func schemaURIsToAny(uris []core.SchemaURI) []any {
	ids := make([]any, len(uris))
	for i, uri := range uris {
		ids[i] = string(uri)
	}
	return ids
}

func checkSize[T core.Resource](limits protocol.Limits, resource T) error {
	encoded, err := json.Marshal(resource)
	if err != nil {
		return scimerrors.ErrInternal("could not encode the resource")
	}
	return checkEncodedSize(limits, encoded)
}

func checkEncodedSize(limits protocol.Limits, encoded []byte) error {
	if limits.MaxResourceBytes > 0 && len(encoded) > limits.MaxResourceBytes {
		return scimerrors.ErrTooLarge("the resource would exceed " + strconv.Itoa(limits.MaxResourceBytes) + " bytes")
	}
	return nil
}

func unchanged(before, after core.Object) bool {
	before, after = maps.Clone(before), maps.Clone(after)
	before.Remove("meta")
	after.Remove("meta")
	return reflect.DeepEqual(before, after)
}
