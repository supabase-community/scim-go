package server

import "github.com/supabase-community/scim-go/pkg/core"

type Resource[T Entity] struct {
	name        string
	endpoint    string
	id          core.SchemaURI
	description string
	attributes  core.Attributes
	extensions  []extension
	repository  Repository[T]
}

func NewResource[T Entity](name, endpoint string, id core.SchemaURI, attributes ...*core.Attribute) *Resource[T] {
	return &Resource[T]{
		name:       name,
		endpoint:   endpoint,
		id:         id,
		attributes: attributes,
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

type extension struct {
	id         core.SchemaURI
	attributes core.Attributes
}

// WithExtension adds a schema extension to the resource type, per RFC 7643, Section 6.
func (c *Resource[T]) WithExtension(id core.SchemaURI, attributes ...*core.Attribute) *Resource[T] {
	c.extensions = append(c.extensions, extension{id: id, attributes: attributes})
	return c
}

func (c *Resource[T]) schema(basePath string) *core.Schema {
	return core.NewSchema(c.id).
		WithName(core.ResourceTypeName(c.name)).
		WithDescription(c.description).
		WithLocation(basePath + "/Schemas/" + string(c.id)).
		With(c.attributes...)
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

func (c *Resource[T]) schemas(basePath string) core.Schemas {
	schemas := make(core.Schemas, 1, 1+len(c.extensions))
	schemas[0] = c.schema(basePath)
	for _, extension := range c.extensions {
		schemas = append(schemas, core.NewSchema(extension.id).
			WithLocation(basePath+"/Schemas/"+string(extension.id)).
			With(extension.attributes...))
	}
	return schemas
}

func (c *Resource[T]) mount(s *Server, schemas core.Schemas) {
	path := s.basePath + c.endpoint
	repository := c.repository
	if repository == nil {
		repository = NewRepository[T](path, schemas)
	}
	service := NewService(repository, validators(schemas, repository)...)
	controller := NewController(service, schemas, s.limits, s.config)

	s.mux.HandleFunc("GET "+path, s.handle(controller.List))
	s.mux.HandleFunc("POST "+path, s.handle(controller.Create))
	s.mux.HandleFunc("GET "+path+"/{id}", s.handle(controller.ByID))
	s.mux.HandleFunc("PUT "+path+"/{id}", s.handle(controller.Replace))
	s.mux.HandleFunc("PATCH "+path+"/{id}", s.handle(controller.Patch))
	s.mux.HandleFunc("DELETE "+path+"/{id}", s.handle(controller.Delete))
}
