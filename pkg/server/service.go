package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/supabase-community/scim-go/internal/decode"
	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

var errUnchanged = errors.New("server: the patch changes nothing")

type Service[T core.Resource] interface {
	List(ctx context.Context, query *protocol.SearchRequest) (items []T, total int, err error)
	Get(ctx context.Context, id string) (T, error)
	Create(ctx context.Context, document core.Object) (T, error)
	Replace(ctx context.Context, req *protocol.ReplaceRequest) (T, error)
	Patch(ctx context.Context, req *protocol.PatchRequest) (T, error)
	Delete(ctx context.Context, req *protocol.DeleteRequest) error
}

type Validator[T core.Resource] func(ctx context.Context, candidate T) error

type service[T core.Resource] struct {
	repo            Repository[T]
	schemas         core.Schemas
	limits          Limits
	characteristics func(existing, candidate core.Object) error
	validators      []Validator[T]
}

type revision[T core.Resource] struct {
	existing  core.Object
	candidate core.Object
	resource  T
}

type snapshot[T core.Resource] struct {
	resource T
	object   core.Object
	encoded  []byte
}

type change[T core.Resource] func(before snapshot[T]) (revision[T], error)

func NewService[T core.Resource](repo Repository[T], schemas core.Schemas, limits Limits, validators ...Validator[T]) Service[T] {
	return &service[T]{repo: repo, schemas: schemas, limits: limits, characteristics: newCharacteristics(schemas), validators: validators}
}

func (s *service[T]) Get(ctx context.Context, id string) (T, error) {
	return s.repo.Read(ctx, id)
}

func (s *service[T]) List(ctx context.Context, query *protocol.SearchRequest) ([]T, int, error) {
	return s.repo.List(ctx, query)
}

func (s *service[T]) Create(ctx context.Context, document core.Object) (T, error) {
	var zero T
	item, err := protocol.ResourceFrom[T](document, nil, s.schemas)
	if err != nil {
		return zero, err
	}
	stampSchemas(s.schemas, item, document)
	if err := s.admit(ctx, revision[T]{candidate: document, resource: item}); err != nil {
		return zero, err
	}
	return s.repo.Create(ctx, item)
}

func (s *service[T]) Replace(ctx context.Context, req *protocol.ReplaceRequest) (T, error) {
	return s.write(ctx, req.ID, req.Version, func(before snapshot[T]) (revision[T], error) {
		return s.replaced(before.object, req)
	})
}

func (s *service[T]) Delete(ctx context.Context, req *protocol.DeleteRequest) error {
	current, err := s.current(ctx, req.ID, req.Version)
	if err != nil {
		return err
	}
	return conflict(req.Version, s.repo.Delete(ctx, current))
}

func (s *service[T]) Patch(ctx context.Context, req *protocol.PatchRequest) (T, error) {
	return s.write(ctx, req.ID, req.Version, func(before snapshot[T]) (revision[T], error) {
		return s.patched(before, req)
	})
}

func (s *service[T]) write(ctx context.Context, id, version string, apply change[T]) (T, error) {
	var zero T
	current, err := s.current(ctx, id, version)
	if err != nil {
		return zero, err
	}
	before, err := snapshotOf(current)
	if err != nil {
		return zero, err
	}
	next, err := apply(before)
	if errors.Is(err, errUnchanged) {
		return current, nil
	}
	if err != nil {
		return zero, err
	}
	return s.save(ctx, version, current, next)
}

func (s *service[T]) save(ctx context.Context, version string, current T, next revision[T]) (T, error) {
	var zero T
	if err := s.admit(ctx, next); err != nil {
		return zero, err
	}
	common := next.resource.Common()
	common.ID, common.Meta = current.Common().ID, current.Common().Meta
	saved, err := s.repo.Update(ctx, next.resource)
	return saved, conflict(version, err)
}

// RFC 7644 Section 3.14: a stale If-Match fails before any work; the repository rechecks the version on write.
func (s *service[T]) current(ctx context.Context, id, version string) (T, error) {
	current, err := s.repo.Read(ctx, id)
	if err != nil {
		return current, err
	}
	if version != "" && current.Common().Meta.Version != version {
		var zero T
		return zero, scimerrors.ErrPreconditionFailed("resource has changed on the server")
	}
	return current, nil
}

func (s *service[T]) patched(before snapshot[T], req *protocol.PatchRequest) (revision[T], error) {
	document := before.object
	patched, err := req.Patch[T](document, s.schemas, patch.MaxFilterEvaluations(s.limits.MaxFilterEvaluations), patch.MaxWriteBytes(s.limits.MaxWriteBytes))
	if err != nil {
		return revision[T]{}, err
	}
	patched.Common().Meta = before.resource.Common().Meta
	stampSchemas(s.schemas, patched, document)
	// RFC 7644 Section 3.5.2.1: a no-op SHALL NOT change the modify timestamp.
	return revision[T]{candidate: document, resource: patched}, unchanged(before.encoded, patched)
}

func (s *service[T]) replaced(existing core.Object, req *protocol.ReplaceRequest) (revision[T], error) {
	resource, err := req.Replace[T](existing, s.schemas)
	if err != nil {
		return revision[T]{}, err
	}
	stampSchemas(s.schemas, resource, req.Attributes)
	return revision[T]{existing: existing, candidate: req.Attributes, resource: resource}, nil
}

func (s *service[T]) admit(ctx context.Context, next revision[T]) error {
	if err := s.characteristics(next.existing, next.candidate); err != nil {
		return err
	}
	for _, validate := range s.validators {
		if err := validate(ctx, next.resource); err != nil {
			return err
		}
	}
	return nil
}

// stampSchemas lists the base schema plus every extension with assigned data in object, per RFC 7643, Section 3.
func stampSchemas[T core.Resource](schemas core.Schemas, resource T, object core.Object) {
	uris := []core.SchemaURI{schemas.Base().ID}
	for _, extension := range schemas.Extensions() {
		if !value.IsUnassigned(object.Get(string(extension.ID))) {
			uris = append(uris, extension.ID)
		}
	}
	resource.Common().Schemas = uris
	object.Set("schemas", schemaURIsToAny(uris))
}

func schemaURIsToAny(uris []core.SchemaURI) []any {
	ids := make([]any, len(uris))
	for i, uri := range uris {
		ids[i] = string(uri)
	}
	return ids
}

func conflict(version string, err error) error {
	if version == "" && errors.Is(err, scimerrors.ErrPreconditionFailed("")) {
		return scimerrors.NewError(http.StatusConflict, "", "resource changed during the request; retry")
	}
	return err
}

func unchanged[T core.Resource](stored []byte, patched T) error {
	candidate, err := json.Marshal(patched)
	if err != nil {
		return scimerrors.ErrInternal("could not encode the resource")
	}
	if bytes.Equal(stored, candidate) {
		return errUnchanged
	}
	return nil
}

func snapshotOf[T core.Resource](resource T) (snapshot[T], error) {
	encoded, err := json.Marshal(resource)
	if err != nil {
		return snapshot[T]{}, err
	}
	object, err := decode.Object(encoded)
	return snapshot[T]{resource: resource, object: object, encoded: encoded}, err
}
