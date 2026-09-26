package server

import (
	"errors"
	"net/http"
	"reflect"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
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
	limits  protocol.Limits
	config  *core.ServiceProviderConfig
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
	items, total, err := c.service.List(r.Context(), query)
	if err != nil {
		return protocol.SendError(w, err)
	}
	resources := projection.All(items)
	return protocol.Send(w, http.StatusOK, protocol.NewListResponse(query.StartIndex, total, resources))
}

func (c *controller[T]) ByID(w http.ResponseWriter, r *http.Request) error {
	projection, err := protocol.ParseProjection(r.URL.Query(), c.schemas)
	if err != nil {
		return protocol.SendError(w, err)
	}
	resource, err := c.service.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		return protocol.SendError(w, err)
	}
	c.setVersion(w, resource)
	return c.send(w, http.StatusOK, resource, projection)
}

func (c *controller[T]) Create(w http.ResponseWriter, r *http.Request) error {
	projection, err := protocol.ParseProjection(r.URL.Query(), c.schemas)
	if err != nil {
		return protocol.SendError(w, err)
	}
	resource, err := protocol.DecodeResource[T](r.Body, nil, c.schemas)
	if err != nil {
		return protocol.SendError(w, err)
	}
	if err := c.stampSchemas(resource); err != nil {
		return protocol.SendError(w, err)
	}
	created, err := c.service.Create(r.Context(), resource)
	if err != nil {
		return protocol.SendError(w, err)
	}
	w.Header().Set("Location", created.Common().Meta.Location)
	c.setVersion(w, created)
	return c.send(w, http.StatusCreated, created, projection)
}

func (c *controller[T]) Replace(w http.ResponseWriter, r *http.Request) error {
	projection, err := protocol.ParseProjection(r.URL.Query(), c.schemas)
	if err != nil {
		return protocol.SendError(w, err)
	}
	existing, err := c.service.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		return protocol.SendError(w, err)
	}
	resource, err := protocol.DecodeResource[T](r.Body, existing, c.schemas)
	if err != nil {
		return protocol.SendError(w, err)
	}
	resource.Common().Meta = core.Meta{Version: c.ifMatch(r)}
	if err := c.stampSchemas(resource); err != nil {
		return protocol.SendError(w, err)
	}
	replaced, err := c.service.Replace(r.Context(), resource)
	if err != nil {
		return protocol.SendError(w, err)
	}
	c.setVersion(w, replaced)
	return c.send(w, http.StatusOK, replaced, projection)
}

func (c *controller[T]) Patch(w http.ResponseWriter, r *http.Request) error {
	if !c.config.SupportsPatch() {
		return protocol.SendError(w, scimerrors.ErrNotImplemented(`"PATCH" is not supported`))
	}
	projection, err := protocol.ParseProjection(r.URL.Query(), c.schemas)
	if err != nil {
		return protocol.SendError(w, err)
	}
	existing, err := c.service.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		return protocol.SendError(w, err)
	}
	if match := c.ifMatch(r); match != "" && existing.Common().Meta.Version != match {
		return protocol.SendError(w, scimerrors.ErrPreconditionFailed("resource has changed on the server"))
	}
	req, err := c.limits.DecodePatchRequest(r.Body)
	if err != nil {
		return protocol.SendError(w, err)
	}
	patched, err := req.PatchWithin(existing, c.schemas, c.limits.MaxFilterEvaluations)
	if err != nil {
		return protocol.SendError(w, err)
	}
	patched.Common().Meta = core.Meta{Version: existing.Common().Meta.Version}
	if err := c.stampSchemas(patched); err != nil {
		return protocol.SendError(w, err)
	}
	replaced, err := c.persist(r, existing, patched)
	if err != nil {
		return protocol.SendError(w, err)
	}
	c.setVersion(w, replaced)
	return c.send(w, http.StatusOK, replaced, projection)
}

func (c *controller[T]) Delete(w http.ResponseWriter, r *http.Request) error {
	if err := c.service.Delete(r.Context(), r.PathValue("id"), c.ifMatch(r)); err != nil {
		return protocol.SendError(w, err)
	}
	return protocol.Send(w, http.StatusNoContent, nil)
}

func (c *controller[T]) send(w http.ResponseWriter, status int, resource T, projection protocol.Projection) error {
	setLocation(w, resource.Common().Meta)
	return protocol.Send(w, status, projection.Of(resource))
}

func (c *controller[T]) setVersion(w http.ResponseWriter, resource T) {
	if c.config.SupportsVersioning() {
		w.Header().Set("ETag", resource.Common().Meta.Version)
	}
}

// persist skips the write when the patch changed nothing, per RFC 7644, Section 3.5.2.1: a no-op SHALL NOT change the modify timestamp.
func (c *controller[T]) persist(r *http.Request, existing, patched T) (T, error) {
	same, err := unchanged(existing, patched)
	if err != nil || same {
		return existing, err
	}
	replaced, err := c.service.Replace(r.Context(), patched)
	return replaced, c.lostRace(r, err)
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

// stampSchemas lists the base schema plus every extension with assigned data, per RFC 7643, Section 3.
func (c *controller[T]) stampSchemas(resource T) error {
	object, err := core.NewObject(resource)
	if err != nil {
		return err
	}
	uris := []core.SchemaURI{c.schemas.Base().ID}
	for _, extension := range c.schemas.Extensions() {
		if !value.IsUnassigned(object.Get(string(extension.ID))) {
			uris = append(uris, extension.ID)
		}
	}
	resource.Common().Schemas = uris
	return nil
}

func unchanged(existing, patched any) (bool, error) {
	before, err := core.NewObject(existing)
	if err != nil {
		return false, err
	}
	after, err := core.NewObject(patched)
	if err != nil {
		return false, err
	}
	before.Remove("meta")
	after.Remove("meta")
	return reflect.DeepEqual(before, after), nil
}
