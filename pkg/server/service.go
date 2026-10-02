package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"

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

type service[T core.Resource] struct {
	repo       Repository[T]
	schemas    core.Schemas
	limits     Limits
	validators []Validator[T]
}

func NewService[T core.Resource](repo Repository[T], schemas core.Schemas, limits Limits, validators ...Validator[T]) Service[T] {
	validators = append([]Validator[T]{characteristics[T](schemas)}, validators...)
	return &service[T]{repo: repo, schemas: schemas, limits: limits, validators: validators}
}

func (s *service[T]) Get(ctx context.Context, id string) (T, error) {
	return s.repo.Read(ctx, id)
}

func (s *service[T]) List(ctx context.Context, query *protocol.SearchRequest) ([]T, int, error) {
	return s.repo.List(ctx, query)
}

func (s *service[T]) Create(ctx context.Context, document core.Object) (T, error) {
	item, err := protocol.ResourceFrom[T](document, nil, s.schemas)
	if err != nil {
		return item, err
	}
	if err := s.admit(ctx, nil, stampSchemas(s.schemas, item, document), item); err != nil {
		var zero T
		return zero, err
	}
	return s.repo.Create(ctx, item)
}

func (s *service[T]) Replace(ctx context.Context, req *protocol.ReplaceRequest) (T, error) {
	return s.write(ctx, req.ID, req.Version, func(existing T) (T, error) {
		return s.replaced(ctx, existing, req)
	})
}

func (s *service[T]) Delete(ctx context.Context, req *protocol.DeleteRequest) error {
	current, err := s.current(ctx, req.ID, req.Version)
	if err != nil {
		return err
	}
	return conflict(req.Version, s.repo.Delete(ctx, current))
}

// Patch applies req to the current resource as one unit, per RFC 7644 Section 3.5.2.
func (s *service[T]) Patch(ctx context.Context, req *protocol.PatchRequest) (T, error) {
	return s.write(ctx, req.ID, req.Version, func(existing T) (T, error) {
		return s.patched(ctx, existing, req)
	})
}

func (s *service[T]) write(ctx context.Context, id, version string, change func(existing T) (T, error)) (T, error) {
	var zero T
	current, err := s.current(ctx, id, version)
	if err != nil {
		return zero, err
	}
	next, err := change(current)
	if errors.Is(err, errUnchanged) {
		return current, nil
	}
	if err != nil {
		return zero, err
	}
	next.Common().ID, next.Common().Meta = current.Common().ID, current.Common().Meta
	saved, err := s.repo.Update(ctx, next)
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

func (s *service[T]) patched(ctx context.Context, existing T, req *protocol.PatchRequest) (T, error) {
	var zero T
	stored, before, err := encode(existing)
	if err != nil {
		return zero, err
	}
	after, _ := value.Clone(map[string]any(before)).(map[string]any)
	patched, err := req.Patch[T](after, s.schemas, patch.MaxFilterEvaluations(s.limits.MaxFilterEvaluations), patch.MaxWriteBytes(s.limits.MaxWriteBytes))
	if err != nil {
		return zero, err
	}
	patched.Common().Meta = existing.Common().Meta
	stampSchemas(s.schemas, patched, after)
	// RFC 7644 Section 3.5.2.1: a no-op SHALL NOT change the modify timestamp.
	if err := unchanged(stored, patched); err != nil {
		return patched, err
	}
	return patched, s.admit(ctx, before, after, patched)
}

func (s *service[T]) replaced(ctx context.Context, existing T, req *protocol.ReplaceRequest) (T, error) {
	var zero T
	_, before, err := encode(existing)
	if err != nil {
		return zero, err
	}
	resource, err := req.Replace[T](before, s.schemas)
	if err != nil {
		return zero, err
	}
	return resource, s.admit(ctx, before, stampSchemas(s.schemas, resource, req.Attributes), resource)
}

func (s *service[T]) admit(ctx context.Context, before, after core.Object, resource T) error {
	if before != nil {
		ctx = withExisting(ctx, before)
	}
	return s.validate(withCandidate(ctx, after), resource)
}

func (s *service[T]) validate(ctx context.Context, item T) error {
	for _, validate := range s.validators {
		if err := validate(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

// stampSchemas lists the base schema plus every extension with assigned data in object, per RFC 7643, Section 3.
func stampSchemas[T core.Resource](schemas core.Schemas, resource T, object core.Object) core.Object {
	uris := []core.SchemaURI{schemas.Base().ID}
	for _, extension := range schemas.Extensions() {
		if !value.IsUnassigned(object.Get(string(extension.ID))) {
			uris = append(uris, extension.ID)
		}
	}
	resource.Common().Schemas = uris
	object.Set("schemas", schemaURIsToAny(uris))
	return object
}

func schemaURIsToAny(uris []core.SchemaURI) []any {
	ids := make([]any, len(uris))
	for i, uri := range uris {
		ids[i] = string(uri)
	}
	return ids
}

func encode[T core.Resource](resource T) ([]byte, core.Object, error) {
	raw, err := json.Marshal(resource)
	if err != nil {
		return nil, nil, scimerrors.ErrInternal("could not encode the resource")
	}
	object, err := core.DecodeObject(raw)
	return raw, object, err
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
