package server

import (
	"context"
	"encoding/json"
	"maps"
	"reflect"
	"strconv"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type Service[T core.Resource] interface {
	List(ctx context.Context, query *protocol.SearchRequest) (items []T, total int, err error)
	Get(ctx context.Context, id string) (T, error)
	Create(ctx context.Context, item T) (T, error)
	Replace(ctx context.Context, item T) (T, error)
	Patch(ctx context.Context, cmd PatchCommand) (Patched[T], error)
	Delete(ctx context.Context, id, version string) error
}

type service[T core.Resource] struct {
	repo       Repository[T]
	schemas    core.Schemas
	limits     protocol.Limits
	validators []Validator[T]
	patcher    AttributePatcher[T]
}

type PatchCommand struct {
	ID        string
	Version   string
	Request   *protocol.PatchRequest
	NoContent bool
}

type Patched[T core.Resource] struct {
	Resource T
	Meta     core.Meta
}

func NewService[T core.Resource](repo Repository[T], schemas core.Schemas, limits protocol.Limits, validators ...Validator[T]) Service[T] {
	return &service[T]{repo: repo, schemas: schemas, limits: limits, validators: validators}
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

// Patch applies cmd.Request to the current resource as one unit, per RFC 7644 Section 3.5.2.
func (s *service[T]) Patch(ctx context.Context, cmd PatchCommand) (Patched[T], error) {
	if found, ok := s.delta(cmd); ok {
		meta, _, err := s.patcher.PatchAttribute(ctx, cmd.ID, cmd.Version, AttributeDelta{Attribute: found.attribute, Added: found.added, Removed: found.removed})
		return Patched[T]{Meta: meta}, err
	}
	patched, err := s.patchOnce(ctx, cmd)
	if err != nil {
		return Patched[T]{}, err
	}
	return Patched[T]{Resource: patched, Meta: patched.Common().Meta}, nil
}

func (s *service[T]) delta(cmd PatchCommand) (delta, bool) {
	if s.patcher == nil || !cmd.NoContent {
		return delta{}, false
	}
	return eligibleDelta(s.schemas, cmd.Request.Operations)
}

func (s *service[T]) patchOnce(ctx context.Context, cmd PatchCommand) (T, error) {
	existing, err := s.current(ctx, cmd.ID, cmd.Version)
	if err != nil {
		return existing, err
	}
	patched, after, err := s.apply(existing, cmd.Request)
	if err != nil {
		return existing, err
	}
	return s.persist(ctx, existing, patched, after)
}

// current rejects a stale non-empty version, per RFC 7644 Section 3.14.
func (s *service[T]) current(ctx context.Context, id, version string) (T, error) {
	existing, err := s.repo.Get(ctx, id)
	if err != nil {
		return existing, err
	}
	if version != "" && existing.Common().Meta.Version != version {
		return existing, scimerrors.ErrPreconditionFailed("resource has changed on the server")
	}
	return existing, nil
}

func (s *service[T]) apply(existing T, req *protocol.PatchRequest) (T, core.Object, error) {
	patched, err := req.Patch(existing, s.schemas, patch.MaxFilterEvaluations(s.limits.MaxFilterEvaluations), patch.MaxWriteBytes(s.limits.MaxWriteBytes))
	if err != nil {
		return patched, nil, err
	}
	patched.Common().Meta = core.Meta{Version: existing.Common().Meta.Version}
	after, err := stampSchemas(s.schemas, patched)
	return patched, after, err
}

// persist skips the write when the patch changed nothing, per RFC 7644, Section 3.5.2.1: a no-op SHALL NOT change the modify timestamp.
func (s *service[T]) persist(ctx context.Context, existing, patched T, after core.Object) (T, error) {
	before, err := core.NewObject(existing)
	if err != nil {
		return existing, scimerrors.ErrInternal("could not encode the resource")
	}
	if unchanged(before, after) {
		return existing, nil
	}
	if err := checkSize(s.limits, patched); err != nil {
		return existing, err
	}
	return s.Replace(withCandidate(withExisting(ctx, before), after), patched)
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
