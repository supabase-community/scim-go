package server

import (
	"net/http"
	"slices"

	"github.com/supabase-community/scim-go/pkg/core"
)

type Resource[T Entity] struct {
	name         string
	endpoint     string
	id           core.SchemaURI
	description  string
	fields       Fields[T]
	extensions   []extension[T]
	errorHandler func(error)
	repository   Repository[T]
	service      Service[T]
}

func NewResource[T Entity](name, endpoint string, id core.SchemaURI, fields Fields[T]) *Resource[T] {
	return &Resource[T]{
		name:         name,
		endpoint:     endpoint,
		id:           id,
		fields:       fields,
		errorHandler: func(err error) {},
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

func (c *Resource[T]) WithRepository(repository Repository[T]) *Resource[T] {
	if c.service != nil {
		panic("server: WithRepository cannot be combined with WithService")
	}
	c.repository = repository
	return c
}

func (c *Resource[T]) WithService(service Service[T]) *Resource[T] {
	c.service = service
	return c
}

type extension[T Entity] struct {
	id     core.SchemaURI
	fields Fields[T]
}

// WithExtension adds a schema extension to the resource type, per RFC 7643, Section 6.
func (c *Resource[T]) WithExtension(id core.SchemaURI, fields Fields[T]) *Resource[T] {
	c.extensions = append(c.extensions, extension[T]{id: id, fields: fields})
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
	resourceType := &core.ResourceType{
		Schemas:  []core.SchemaURI{core.SchemaResourceType},
		ID:       core.ResourceTypeName(c.name),
		Name:     core.ResourceTypeName(c.name),
		Endpoint: basePath + c.endpoint,
		Schema:   c.id,
	}
	for _, extension := range c.extensions {
		resourceType.Extend(core.SchemaExtension{Schema: extension.id})
	}
	return resourceType
}

func (c *Resource[T]) schemas(basePath string) []*core.Schema {
	schemas := []*core.Schema{c.schema(basePath)}
	for _, extension := range c.extensions {
		schemas = append(schemas, core.NewSchema(extension.id).
			WithLocation(basePath+"/Schemas/"+string(extension.id)).
			With(extension.fields.Attributes()...))
	}
	return schemas
}

func (c *Resource[T]) allFields() Fields[T] {
	fields := slices.Clone(c.fields)
	for _, extension := range c.extensions {
		fields = append(fields, extension.fields...)
	}
	return fields
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
	service := c.service
	schemas := c.schemas(basePath)
	if service == nil {
		repository := c.repository
		fields := c.allFields()
		accessors := fields.Accessors()
		if repository == nil {
			repository = NewRepository(basePath+c.endpoint, schemas[0], fields)
		}
		service = NewService[T](repository,
			Required(accessors),
			CanonicalValues(accessors),
			Mutability(accessors, repository),
			Uniqueness(accessors, repository),
		)
	}
	return NewController[T](service, schemas, basePath+c.endpoint)
}

func (c *Resource[T]) handle(fn func(http.ResponseWriter, *http.Request) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := fn(w, r); err != nil {
			c.errorHandler(err)
		}
	}
}
