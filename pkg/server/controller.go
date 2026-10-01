package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// Controller handles the HTTP requests for one SCIM resource type, per RFC 7644, Section 3.
type Controller[T core.Resource] interface {
	List(http.ResponseWriter, *http.Request) error
	ByID(http.ResponseWriter, *http.Request) error
	Create(http.ResponseWriter, *http.Request) error
	Replace(http.ResponseWriter, *http.Request) error
	Patch(http.ResponseWriter, *http.Request) error
	Delete(http.ResponseWriter, *http.Request) error
}

type controller[T core.Resource] struct {
	schemas core.Schemas
	service Service[T]
	patcher AttributePatcher[T]
	limits  protocol.Limits
	config  *core.ServiceProviderConfig
}

type decodedPatch[T core.Resource] struct {
	existing T
	req      *protocol.PatchRequest
}

func NewController[T core.Resource](service Service[T], schemas core.Schemas, limits protocol.Limits, config *core.ServiceProviderConfig) Controller[T] {
	return &controller[T]{
		schemas: schemas,
		service: service,
		limits:  limits,
		config:  config,
	}
}

func (c *controller[T]) List(w http.ResponseWriter, r *http.Request) error {
	values := r.URL.Query()
	if values.Get("filter") != "" && !c.config.SupportsFilter() {
		return protocol.SendError(w, scimerrors.ErrNotImplemented(`"filter" is not supported`))
	}
	if values.Get("sortBy") != "" && !c.config.SupportsSort() {
		return protocol.SendError(w, scimerrors.ErrNotImplemented(`"sortBy" is not supported`))
	}
	query, err := c.limits.ParseSearchRequest(values)
	if err != nil {
		return protocol.SendError(w, err)
	}
	if err := query.Validate(c.schemas); err != nil {
		return protocol.SendError(w, err)
	}
	projection, err := query.Projection(c.schemas)
	if err != nil {
		return protocol.SendError(w, err)
	}
	items, total, err := c.service.List(protocol.WithProjection(r.Context(), projection), query)
	if err != nil {
		return protocol.SendError(w, err)
	}
	resources := projection.All(items)
	return protocol.Send(w, http.StatusOK, protocol.NewListResponse(query.StartIndex, total, resources))
}

func (c *controller[T]) ByID(w http.ResponseWriter, r *http.Request) error {
	projection, ok, err := c.projectionFor(w, r)
	if !ok {
		return err
	}
	resource, ok, err := c.existing(w, r.WithContext(protocol.WithProjection(r.Context(), projection)))
	if !ok {
		return err
	}
	c.setVersion(w, resource)
	return c.send(w, http.StatusOK, resource, projection)
}

func (c *controller[T]) Create(w http.ResponseWriter, r *http.Request) error {
	projection, ok, err := c.projectionFor(w, r)
	if !ok {
		return err
	}
	resource, err := protocol.DecodeResource[T](r.Body, nil, c.schemas)
	if err != nil {
		return protocol.SendError(w, err)
	}
	after, ok, err := c.prepare(w, resource)
	if !ok {
		return err
	}
	created, err := c.service.Create(withCandidate(r.Context(), after), resource)
	if err != nil {
		return protocol.SendError(w, err)
	}
	w.Header().Set("Location", created.Common().Meta.Location)
	c.setVersion(w, created)
	return c.send(w, http.StatusCreated, created, projection)
}

func (c *controller[T]) Replace(w http.ResponseWriter, r *http.Request) error {
	projection, ok, err := c.projectionFor(w, r)
	if !ok {
		return err
	}
	existing, ok, err := c.existing(w, r)
	if !ok {
		return err
	}
	resource, err := protocol.DecodeResource[T](r.Body, existing, c.schemas)
	if err != nil {
		return protocol.SendError(w, err)
	}
	resource.Common().Meta = core.Meta{Version: c.version(r, existing)}
	after, ok, err := c.prepare(w, resource)
	if !ok {
		return err
	}
	before, err := core.NewObject(existing)
	if err != nil {
		return protocol.SendError(w, scimerrors.ErrInternal("could not encode the resource"))
	}
	replaced, err := c.service.Replace(withCandidate(withExisting(r.Context(), before), after), resource)
	if err != nil {
		return protocol.SendError(w, c.lostRace(r, err))
	}
	c.setVersion(w, replaced)
	return c.send(w, http.StatusOK, replaced, projection)
}

func (c *controller[T]) version(r *http.Request, existing T) string {
	if match := c.ifMatch(r); match != "" {
		return match
	}
	return existing.Common().Meta.Version
}

func (c *controller[T]) Patch(w http.ResponseWriter, r *http.Request) error {
	if !c.config.SupportsPatch() {
		return protocol.SendError(w, scimerrors.ErrNotImplemented(`"PATCH" is not supported`))
	}
	projection, ok, err := c.projectionFor(w, r)
	if !ok {
		return err
	}
	if c.patcher != nil {
		return c.patchWithDelta(w, r, projection)
	}
	return c.patchByReplacing(w, r, projection)
}

// patchByReplacing is RFC 7644 Section 3.5.2's general case: Get the full resource, apply every operation in-process, and Replace it.
func (c *controller[T]) patchByReplacing(w http.ResponseWriter, r *http.Request, projection protocol.Projection) error {
	req, err := c.limits.DecodePatchRequest(r.Body)
	if err != nil {
		return protocol.SendError(w, err)
	}
	existing, ok, err := c.existingWithIfMatch(w, r)
	if !ok {
		return err
	}
	return c.applyPatch(w, r, projection, decodedPatch[T]{existing, req})
}

func (c *controller[T]) patchWithDelta(w http.ResponseWriter, r *http.Request, projection protocol.Projection) error {
	req, err := c.limits.DecodePatchRequest(r.Body)
	if err != nil {
		return protocol.SendError(w, err)
	}
	found, ok := eligibleDelta(c.schemas, req.Operations)
	if !ok || !noContentEligible(r, c.schemas, req.Operations) {
		existing, ok, err := c.existingWithIfMatch(w, r)
		if !ok {
			return err
		}
		return c.applyPatch(w, r, projection, decodedPatch[T]{existing, req})
	}
	meta, _, err := c.patcher.PatchAttribute(r.Context(), r.PathValue("id"), c.ifMatch(r), AttributeDelta{Attribute: found.attribute, Added: found.added, Removed: found.removed})
	if err != nil {
		return protocol.SendError(w, c.lostRace(r, err))
	}
	c.setETag(w, meta.Version)
	setLocation(w, meta)
	return protocol.Send(w, http.StatusNoContent, nil)
}

func (c *controller[T]) existingWithIfMatch(w http.ResponseWriter, r *http.Request) (existing T, ok bool, err error) {
	existing, ok, err = c.existing(w, r)
	if !ok {
		return existing, false, err
	}
	if match := c.ifMatch(r); match != "" && existing.Common().Meta.Version != match {
		return existing, false, protocol.SendError(w, scimerrors.ErrPreconditionFailed("resource has changed on the server"))
	}
	return existing, true, nil
}

func (c *controller[T]) applyPatch(w http.ResponseWriter, r *http.Request, projection protocol.Projection, d decodedPatch[T]) error {
	existing := d.existing
	patched, err := d.req.Patch(existing, c.schemas, patch.MaxFilterEvaluations(c.limits.MaxFilterEvaluations), patch.MaxWriteBytes(c.limits.MaxWriteBytes))
	if err != nil {
		return protocol.SendError(w, err)
	}
	patched.Common().Meta = core.Meta{Version: existing.Common().Meta.Version}
	after, err := stampSchemas(c.schemas, patched)
	if err != nil {
		return protocol.SendError(w, err)
	}
	replaced, err := c.persist(r, existing, patched, after)
	if err != nil {
		return protocol.SendError(w, err)
	}
	c.setVersion(w, replaced)
	return c.sendPatched(w, replaced, projection, noContentEligible(r, c.schemas, d.req.Operations))
}

// sendPatched returns 204 for a Group PATCH eligible under noContentEligible, or 200 with the resource otherwise, per RFC 7644 Section 3.5.2.
func (c *controller[T]) sendPatched(w http.ResponseWriter, replaced T, projection protocol.Projection, noContent bool) error {
	if noContent {
		setLocation(w, replaced.Common().Meta)
		return protocol.Send(w, http.StatusNoContent, nil)
	}
	return c.send(w, http.StatusOK, replaced, projection)
}

func (c *controller[T]) Delete(w http.ResponseWriter, r *http.Request) error {
	if err := c.service.Delete(r.Context(), r.PathValue("id"), c.ifMatch(r)); err != nil {
		return protocol.SendError(w, err)
	}
	return protocol.Send(w, http.StatusNoContent, nil)
}

func (c *controller[T]) projectionFor(w http.ResponseWriter, r *http.Request) (projection protocol.Projection, ok bool, err error) {
	projection, err = protocol.ParseProjection(r.URL.Query(), c.schemas)
	if err != nil {
		return projection, false, protocol.SendError(w, err)
	}
	return projection, true, nil
}

func (c *controller[T]) existing(w http.ResponseWriter, r *http.Request) (resource T, ok bool, err error) {
	resource, err = c.service.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		return resource, false, protocol.SendError(w, err)
	}
	return resource, true, nil
}

func (c *controller[T]) send(w http.ResponseWriter, status int, resource T, projection protocol.Projection) error {
	setLocation(w, resource.Common().Meta)
	return protocol.Send(w, status, projection.Of(resource))
}

func (c *controller[T]) setVersion(w http.ResponseWriter, resource T) {
	c.setETag(w, resource.Common().Meta.Version)
}

func (c *controller[T]) setETag(w http.ResponseWriter, version string) {
	if c.config.SupportsVersioning() {
		w.Header().Set("ETag", version)
	}
}

// persist skips the write when the patch changed nothing, per RFC 7644, Section 3.5.2.1: a no-op SHALL NOT change the modify timestamp.
func (c *controller[T]) persist(r *http.Request, existing, patched T, after core.Object) (T, error) {
	encoded, err := json.Marshal(patched)
	if err != nil {
		return existing, scimerrors.ErrInternal("could not encode the resource")
	}
	before, err := core.NewObject(existing)
	if err != nil {
		return existing, scimerrors.ErrInternal("could not encode the resource")
	}
	if unchanged(before, after) {
		return existing, nil
	}
	if err := checkEncodedSize(c.limits, encoded); err != nil {
		return existing, err
	}
	replaced, err := c.service.Replace(withCandidate(withExisting(r.Context(), before), after), patched)
	return replaced, c.lostRace(r, err)
}

// prepare stamps schemas into resource and rejects it once it crosses MaxResourceBytes, per RFC 7643, Section 3.
func (c *controller[T]) prepare(w http.ResponseWriter, resource T) (after core.Object, ok bool, err error) {
	after, err = stampSchemas(c.schemas, resource)
	if err != nil {
		return after, false, protocol.SendError(w, err)
	}
	if err := checkSize(c.limits, resource); err != nil {
		return after, false, protocol.SendError(w, err)
	}
	return after, true, nil
}

func (c *controller[T]) lostRace(r *http.Request, err error) error {
	if c.ifMatch(r) == "" && errors.Is(err, scimerrors.ErrPreconditionFailed("")) {
		return scimerrors.NewError(http.StatusConflict, "", "resource changed during the patch; retry")
	}
	return err
}

func (c *controller[T]) ifMatch(r *http.Request) string {
	match := r.Header.Get("If-Match")
	if !c.config.SupportsVersioning() || match == "*" {
		return ""
	}
	return match
}
