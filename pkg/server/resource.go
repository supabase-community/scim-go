package server

import (
	"slices"

	"github.com/supabase-community/scim-go/pkg/core"
)

type Resource[T Entity] struct {
	name        string
	endpoint    string
	id          core.SchemaURI
	description string
	fields      Fields[T]
	extensions  []extension[T]
	repository  Repository[T]
}

func NewResource[T Entity](name, endpoint string, id core.SchemaURI, fields Fields[T]) *Resource[T] {
	return &Resource[T]{
		name:     name,
		endpoint: endpoint,
		id:       id,
		fields:   fields,
	}
}

func (c *Resource[T]) WithDescription(description string) *Resource[T] {
	c.description = description
	return c
}

// WithRepository sets the datastore of the resource type; the server validates every write before it.
func (c *Resource[T]) WithRepository(repository Repository[T]) *Resource[T] {
	c.repository = repository
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

func (c *Resource[T]) mount(s *Server) {
	schemas := c.schemas(s.basePath)
	fields := c.allFields()
	path := s.basePath + c.endpoint
	if c.repository == nil {
		c.repository = NewRepository(path, schemas[0], fields)
	}
	service := NewService(c.repository, Validators(fields, c.repository)...)
	controller := NewController(service, schemas, path, s.limits)

	s.mux.HandleFunc("GET "+path, s.handle(controller.List))
	s.mux.HandleFunc("POST "+path, s.handle(controller.Create))
	s.mux.HandleFunc("GET "+path+"/{id}", s.handle(controller.ByID))
	s.mux.HandleFunc("PUT "+path+"/{id}", s.handle(controller.Replace))
	s.mux.HandleFunc("PATCH "+path+"/{id}", s.handle(controller.Patch))
	s.mux.HandleFunc("DELETE "+path+"/{id}", s.handle(controller.Delete))
}
