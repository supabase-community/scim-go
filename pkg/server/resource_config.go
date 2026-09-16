package server

import (
	"net/http"

	"github.com/supabase-community/scim-go/pkg/core"
)

// ResourceConfig registers one resource type with a Server. Whichever of
// Repository/Service/Controller isn't set via With* falls back to the
// package default at mount time.
type ResourceConfig[T core.Identifiable] struct {
	name         string
	endpoint     string
	schema       *core.Schema
	extraSchemas []*core.Schema
	repository   Repository[T]
	service      Service[T]
	controller   Controller[T]
	validators   []Validator[T]
}

// NewResource declares a resource type named name, mounted at endpoint (e.g. "/Users"), described by schema.
func NewResource[T core.Identifiable](name, endpoint string, schema *core.Schema) *ResourceConfig[T] {
	return &ResourceConfig[T]{name: name, endpoint: endpoint, schema: schema}
}

func (c *ResourceConfig[T]) WithRepository(repository Repository[T]) *ResourceConfig[T] {
	c.repository = repository
	return c
}

func (c *ResourceConfig[T]) WithService(service Service[T]) *ResourceConfig[T] {
	c.service = service
	return c
}

func (c *ResourceConfig[T]) WithController(controller Controller[T]) *ResourceConfig[T] {
	c.controller = controller
	return c
}

func (c *ResourceConfig[T]) WithValidator(validator Validator[T]) *ResourceConfig[T] {
	c.validators = append(c.validators, validator)
	return c
}

// WithSchemas registers additional schemas (e.g. schema extensions) alongside this resource's primary schema.
func (c *ResourceConfig[T]) WithSchemas(extra ...*core.Schema) *ResourceConfig[T] {
	c.extraSchemas = append(c.extraSchemas, extra...)
	return c
}

func (c *ResourceConfig[T]) resourceType(basePath string) *core.ResourceType {
	return &core.ResourceType{
		Schemas:  []core.SchemaURI{core.SchemaResourceType},
		ID:       core.ResourceTypeName(c.name),
		Name:     core.ResourceTypeName(c.name),
		Endpoint: basePath + c.endpoint,
		Schema:   c.schema.ID,
	}
}

func (c *ResourceConfig[T]) schemas() []*core.Schema {
	return append([]*core.Schema{c.schema}, c.extraSchemas...)
}

func (c *ResourceConfig[T]) mount(mux *http.ServeMux, basePath string) {
	repo := c.repository
	if repo == nil {
		repo = NewMemoryRepository[T]([]*core.Schema{c.schema})
	}

	svc := c.service
	if svc == nil {
		svc = NewDefaultService[T](repo, c.schema, c.validators...)
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
