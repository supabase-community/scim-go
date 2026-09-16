package server

import (
	"encoding/json"
	"net/http"

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
	path    string
	schema  *core.Schema
	service Service[T]
}

func NewController[T core.Resource](service Service[T], schema *core.Schema, path string) Controller[T] {
	return &controller[T]{
		path:    path,
		schema:  schema,
		service: service,
	}
}

func (c *controller[T]) List(w http.ResponseWriter, r *http.Request) error {
	query, err := protocol.DefaultLimits.ParseSearchRequest(r.URL.Query())
	if err != nil {
		return protocol.SendError(w, err)
	}
	items, total, err := c.service.List(r.Context(), query)
	if err != nil {
		return protocol.SendError(w, err)
	}
	return protocol.Send(w, http.StatusOK, protocol.NewListResponse(query.StartIndex, total, items))
}

func (c *controller[T]) ByID(w http.ResponseWriter, r *http.Request) error {
	resource, err := c.service.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		return protocol.SendError(w, err)
	}
	return protocol.Send(w, http.StatusOK, resource)
}

func (c *controller[T]) Create(w http.ResponseWriter, r *http.Request) error {
	var resource T
	if err := json.NewDecoder(r.Body).Decode(&resource); err != nil {
		return protocol.SendError(w, scimerrors.ErrInvalidSyntax("request body is not valid JSON"))
	}
	created, err := c.service.Create(r.Context(), resource)
	if err != nil {
		return protocol.SendError(w, err)
	}
	w.Header().Set("Location", c.path+"/"+created.ResourceID())
	return protocol.Send(w, http.StatusCreated, created)
}

func (c *controller[T]) Replace(w http.ResponseWriter, r *http.Request) error {
	var resource T
	if err := json.NewDecoder(r.Body).Decode(&resource); err != nil {
		return protocol.SendError(w, scimerrors.ErrInvalidSyntax("request body is not valid JSON"))
	}
	replaced, err := c.service.Replace(r.Context(), r.PathValue("id"), resource)
	if err != nil {
		return protocol.SendError(w, err)
	}
	return protocol.Send(w, http.StatusOK, replaced)
}

func (c *controller[T]) Patch(w http.ResponseWriter, r *http.Request) error {
	id := r.PathValue("id")
	resource, err := c.service.Get(r.Context(), id)
	if err != nil {
		return protocol.SendError(w, err)
	}
	var req protocol.PatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return protocol.SendError(w, scimerrors.ErrInvalidSyntax("request body is not valid JSON"))
	}
	if err := req.Apply(resource, []*core.Schema{c.schema}); err != nil {
		return protocol.SendError(w, err)
	}
	replaced, err := c.service.Replace(r.Context(), id, resource)
	if err != nil {
		return protocol.SendError(w, err)
	}
	return protocol.Send(w, http.StatusOK, replaced)
}

func (c *controller[T]) Delete(w http.ResponseWriter, r *http.Request) error {
	if err := c.service.Delete(r.Context(), r.PathValue("id")); err != nil {
		return protocol.SendError(w, err)
	}
	return protocol.Send(w, http.StatusNoContent, nil)
}
