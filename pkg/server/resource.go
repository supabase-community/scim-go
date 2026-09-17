package server

import (
	"log"
	"net/http"

	"github.com/supabase-community/scim-go/pkg/core"
)

type Resource[T Entity] struct {
	name     string
	endpoint string
	schema   *core.Schema
}

func NewResource[T Entity](name, endpoint string, schema *core.Schema) *Resource[T] {
	return &Resource[T]{
		name:     name,
		endpoint: endpoint,
		schema:   schema,
	}
}

func (c *Resource[T]) resourceType(basePath string) *core.ResourceType {
	return &core.ResourceType{
		Schemas:  []core.SchemaURI{core.SchemaResourceType},
		ID:       core.ResourceTypeName(c.name),
		Name:     core.ResourceTypeName(c.name),
		Endpoint: basePath + c.endpoint,
		Schema:   c.schema.ID,
	}
}

func (c *Resource[T]) schemas() []*core.Schema {
	return []*core.Schema{c.schema}
}

func (c *Resource[T]) mount(mux *http.ServeMux, basePath string) {
	controller := c.build(basePath)

	path := basePath + c.endpoint
	mux.HandleFunc("GET "+path, handle(controller.List))
	mux.HandleFunc("POST "+path, handle(controller.Create))
	mux.HandleFunc("GET "+path+"/{id}", handle(controller.ByID))
	mux.HandleFunc("PUT "+path+"/{id}", handle(controller.Replace))
	mux.HandleFunc("PATCH "+path+"/{id}", handle(controller.Patch))
	mux.HandleFunc("DELETE "+path+"/{id}", handle(controller.Delete))
}

func (c *Resource[T]) build(basePath string) Controller[T] {
	repository := NewRepository[T](c.schema, NewVisitor[T](map[string]getter[T]{}))
	service := NewService[T](repository, c.schema)
	return NewController[T](service, c.schema, basePath+c.endpoint)
}

func handle(fn func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			log.Printf("%v\n", err)
		}
	}
}
