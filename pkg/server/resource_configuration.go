package server

import (
	"net/http"

	"github.com/supabase-community/scim-go/pkg/core"
)

type ResourceConfiguration[T core.Identifiable] struct {
	name         string
	endpoint     string
	schema       *core.Schema
	extraSchemas []*core.Schema
	repository   Repository[T]
	service      Service[T]
	controller   Controller[T]
	validators   []Validator[T]
}

func NewResource[T core.Identifiable](name, endpoint string, schema *core.Schema) *ResourceConfiguration[T] {
	return &ResourceConfiguration[T]{
		name:     name,
		endpoint: endpoint,
		schema:   schema,
	}
}

func (c *ResourceConfiguration[T]) WithRepository(repository Repository[T]) *ResourceConfiguration[T] {
	c.repository = repository
	return c
}

func (c *ResourceConfiguration[T]) WithService(service Service[T]) *ResourceConfiguration[T] {
	c.service = service
	return c
}

func (c *ResourceConfiguration[T]) WithController(controller Controller[T]) *ResourceConfiguration[T] {
	c.controller = controller
	return c
}

func (c *ResourceConfiguration[T]) WithValidator(validator Validator[T]) *ResourceConfiguration[T] {
	c.validators = append(c.validators, validator)
	return c
}

func (c *ResourceConfiguration[T]) WithSchemas(extra ...*core.Schema) *ResourceConfiguration[T] {
	c.extraSchemas = append(c.extraSchemas, extra...)
	return c
}

func (c *ResourceConfiguration[T]) resourceType(basePath string) *core.ResourceType {
	return &core.ResourceType{
		Schemas:  []core.SchemaURI{core.SchemaResourceType},
		ID:       core.ResourceTypeName(c.name),
		Name:     core.ResourceTypeName(c.name),
		Endpoint: basePath + c.endpoint,
		Schema:   c.schema.ID,
	}
}

func (c *ResourceConfiguration[T]) schemas() []*core.Schema {
	return append([]*core.Schema{c.schema}, c.extraSchemas...)
}

func (c *ResourceConfiguration[T]) mount(mux *http.ServeMux, basePath string) {
	if c.repository == nil {
		c.repository = NewMemoryRepository[T]([]*core.Schema{c.schema})
	}

	svc := c.service
	if svc == nil {
		svc = NewDefaultService[T](c.repository, c.schema, c.validators...)
	}

	path := basePath + c.endpoint
	ctrl := c.controller
	if ctrl == nil {
		ctrl = NewDefaultController[T](svc, c.schema, path)
	}

	mux.HandleFunc("GET "+path, handle(ctrl.List))
	mux.HandleFunc("POST "+path, handle(ctrl.Create))
	mux.HandleFunc("GET "+path+"/{id}", handle(ctrl.ByID))
	mux.HandleFunc("PUT "+path+"/{id}", handle(ctrl.Replace))
	mux.HandleFunc("PATCH "+path+"/{id}", handle(ctrl.Patch))
	mux.HandleFunc("DELETE "+path+"/{id}", handle(ctrl.Delete))
}
