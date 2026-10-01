package server

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"maps"
	"math/big"
	"reflect"
	"strconv"
	"time"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

const retryBackoff = 2 * time.Millisecond

var errUnchanged = errors.New("server: the patch changes nothing")

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
	limits     Limits
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

func NewService[T core.Resource](repo Repository[T], schemas core.Schemas, limits Limits, validators ...Validator[T]) Service[T] {
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

func (s *service[T]) Delete(ctx context.Context, id, version string) error {
	return s.repo.Delete(ctx, id, version)
}

// Patch applies cmd.Request to the current resource as one unit, per RFC 7644 Section 3.5.2.
func (s *service[T]) Patch(ctx context.Context, cmd PatchCommand) (Patched[T], error) {
	if found, ok := s.delta(cmd); ok {
		meta, _, err := s.patcher.PatchAttribute(ctx, cmd.ID, cmd.Version, found)
		return Patched[T]{Meta: meta}, err
	}
	patched, err := s.patchWithRetry(ctx, cmd)
	if err != nil {
		return Patched[T]{}, err
	}
	return Patched[T]{Resource: patched, Meta: patched.Common().Meta}, nil
}

func (s *service[T]) delta(cmd PatchCommand) (AttributeDelta, bool) {
	if s.patcher == nil || !cmd.NoContent {
		return AttributeDelta{}, false
	}
	return eligibleDelta(s.schemas, cmd.Request.Operations)
}

// patchWithRetry reapplies a versionless PATCH to the current resource after a lost race, since RFC 7644 Section 3.14 leaves the version to the client.
func (s *service[T]) patchWithRetry(ctx context.Context, cmd PatchCommand) (T, error) {
	patched, err := s.patchOnce(ctx, cmd)
	for retry := range s.limits.PatchRetries {
		if !retryable(cmd, err) || !pause(ctx, retry) {
			break
		}
		patched, err = s.patchOnce(ctx, cmd)
	}
	return patched, err
}

func (s *service[T]) patchOnce(ctx context.Context, cmd PatchCommand) (T, error) {
	var current T
	patched, err := s.repo.Update(ctx, cmd.ID, cmd.Version, func(existing T) (T, error) {
		current = existing
		return s.patch(ctx, existing, cmd.Request)
	})
	if errors.Is(err, errUnchanged) {
		return current, nil
	}
	return patched, err
}

// patch returns errUnchanged when the request changes nothing, per RFC 7644, Section 3.5.2.1: a no-op SHALL NOT change the modify timestamp.
func (s *service[T]) patch(ctx context.Context, existing T, req *protocol.PatchRequest) (T, error) {
	patched, after, err := s.apply(existing, req)
	if err != nil {
		return patched, err
	}
	before, err := core.NewObject(existing)
	if err != nil {
		return patched, scimerrors.ErrInternal("could not encode the resource")
	}
	if unchanged(before, after) {
		return patched, errUnchanged
	}
	if err := checkSize(s.limits, patched); err != nil {
		return patched, err
	}
	return patched, s.validate(withCandidate(withExisting(ctx, before), after), patched)
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

func checkSize[T core.Resource](limits Limits, resource T) error {
	encoded, err := json.Marshal(resource)
	if err != nil {
		return scimerrors.ErrInternal("could not encode the resource")
	}
	return checkEncodedSize(limits, encoded)
}

func checkEncodedSize(limits Limits, encoded []byte) error {
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

func retryable(cmd PatchCommand, err error) bool {
	return cmd.Version == "" && isPreconditionFailed(err)
}

func isPreconditionFailed(err error) bool {
	return errors.Is(err, scimerrors.ErrPreconditionFailed(""))
}

func pause(ctx context.Context, retry int) bool {
	if ctx.Err() != nil {
		return false
	}
	timer := time.NewTimer(jitter(retryBackoff << min(retry, 4)))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func jitter(limit time.Duration) time.Duration {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(limit)))
	if err != nil {
		return limit
	}
	return time.Duration(n.Int64())
}
