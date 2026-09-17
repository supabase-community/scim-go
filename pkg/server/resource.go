package server

import (
	"log"
	"net/http"

	"github.com/supabase-community/scim-go/pkg/core"
)

type Resource[T Entity] struct {
	name         string
	endpoint     string
	id           core.SchemaURI
	description  string
	fields       Fields[T]
	errorHandler func(error)
}

func NewResource[T Entity](name, endpoint string, id core.SchemaURI, fields Fields[T]) *Resource[T] {
	return &Resource[T]{
		name:     name,
		endpoint: endpoint,
		id:       id,
		fields:   fields,
		errorHandler: func(err error) {
			log.Printf("%v\n", err)
		},
	}
}

func (c *Resource[T]) WithDescription(description string) *Resource[T] {
	c.description = description
	return c
}

func (c *Resource[T]) WithErrorHandler(fn func(error)) *Resource[T] {
	c.errorHandler = fn
	return c
}

func (c *Resource[T]) schema(basePath string) *core.Schema {
	return core.NewSchema(c.id).
		WithName(core.ResourceTypeName(c.name)).
		WithDescription(c.description).
		WithLocation(basePath + "/Schemas/" + string(c.id)).
		With(c.fields.Attributes()...)
}

func (c *Resource[T]) resourceType(basePath string) *core.ResourceType {
	return &core.ResourceType{
		Schemas:  []core.SchemaURI{core.SchemaResourceType},
		ID:       core.ResourceTypeName(c.name),
		Name:     core.ResourceTypeName(c.name),
		Endpoint: basePath + c.endpoint,
		Schema:   c.id,
	}
}

func (c *Resource[T]) schemas(basePath string) []*core.Schema {
	return []*core.Schema{c.schema(basePath)}
}

func (c *Resource[T]) mount(mux *http.ServeMux, basePath string) {
	controller := c.build(basePath)

	path := basePath + c.endpoint
	mux.HandleFunc("GET "+path, c.handle(controller.List))
	mux.HandleFunc("POST "+path, c.handle(controller.Create))
	mux.HandleFunc("GET "+path+"/{id}", c.handle(controller.ByID))
	mux.HandleFunc("PUT "+path+"/{id}", c.handle(controller.Replace))
	mux.HandleFunc("PATCH "+path+"/{id}", c.handle(controller.Patch))
	mux.HandleFunc("DELETE "+path+"/{id}", c.handle(controller.Delete))
}

func (c *Resource[T]) build(basePath string) Controller[T] {
	repository := NewRepository[T](c.schema(basePath), NewVisitor[T](c.fields.Accessors()))
	service := NewService[T](repository, c.schema(basePath))
	return NewController[T](service, c.schema(basePath), basePath+c.endpoint)
}

func (c *Resource[T]) handle(fn func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			c.errorHandler(err)
		}
	}
}
