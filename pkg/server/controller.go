package server

import (
	"net/http"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// Controller handles the HTTP requests for one SCIM resource type, per RFC 7644, Section 3.
type Controller[T Entity] interface {
	List(http.ResponseWriter, *http.Request) error
	ByID(http.ResponseWriter, *http.Request) error
	Create(http.ResponseWriter, *http.Request) error
	Replace(http.ResponseWriter, *http.Request) error
	Patch(http.ResponseWriter, *http.Request) error
	Delete(http.ResponseWriter, *http.Request) error
}

type controller[T Entity] struct {
	schemas []*core.Schema
	service Service[T]
	limits  protocol.Limits
}

func NewController[T Entity](service Service[T], schemas []*core.Schema, limits protocol.Limits) Controller[T] {
	return &controller[T]{
		schemas: schemas,
		service: service,
		limits:  limits,
	}
}

func (c *controller[T]) List(w http.ResponseWriter, r *http.Request) error {
	query, err := c.limits.ParseSearchRequest(r.URL.Query())
	if err != nil {
		return protocol.SendError(w, err)
	}
	items, total, err := c.service.List(r.Context(), query)
	if err != nil {
		return protocol.SendError(w, err)
	}
	resources := query.Projection(c.schemas).All(items)
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
	w.Header().Set("ETag", resource.GetMeta().Version)
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
	created, err := c.service.Create(r.Context(), resource)
	if err != nil {
		return protocol.SendError(w, err)
	}
	w.Header().Set("Location", created.GetMeta().Location)
	w.Header().Set("ETag", created.GetMeta().Version)
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
	resource.SetMeta(core.Meta{Version: r.Header.Get("If-Match")})
	replaced, err := c.service.Replace(r.Context(), resource)
	if err != nil {
		return protocol.SendError(w, err)
	}
	w.Header().Set("ETag", replaced.GetMeta().Version)
	return c.send(w, http.StatusOK, replaced, projection)
}

func (c *controller[T]) Patch(w http.ResponseWriter, r *http.Request) error {
	projection, err := protocol.ParseProjection(r.URL.Query(), c.schemas)
	if err != nil {
		return protocol.SendError(w, err)
	}
	existing, err := c.service.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		return protocol.SendError(w, err)
	}
	if match := r.Header.Get("If-Match"); match != "" && existing.GetMeta().Version != match {
		return protocol.SendError(w, scimerrors.ErrPreconditionFailed("resource has changed on the server"))
	}
	req, err := protocol.DecodePatchRequest(r.Body)
	if err != nil {
		return protocol.SendError(w, err)
	}
	patched, err := req.Patch(existing, c.schemas)
	if err != nil {
		return protocol.SendError(w, err)
	}
	replaced, err := c.service.Replace(r.Context(), patched)
	if err != nil {
		return protocol.SendError(w, err)
	}
	w.Header().Set("ETag", replaced.GetMeta().Version)
	return c.send(w, http.StatusOK, replaced, projection)
}

func (c *controller[T]) Delete(w http.ResponseWriter, r *http.Request) error {
	if err := c.service.Delete(r.Context(), r.PathValue("id"), r.Header.Get("If-Match")); err != nil {
		return protocol.SendError(w, err)
	}
	return protocol.Send(w, http.StatusNoContent, nil)
}

func (c *controller[T]) send(w http.ResponseWriter, status int, resource T, projection protocol.Projection) error {
	return protocol.Send(w, status, projection.Of(resource))
}
