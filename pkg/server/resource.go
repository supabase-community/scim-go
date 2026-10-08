package server

import "github.com/supabase-community/scim-go/pkg/core"

type Resource[T core.Resource] struct {
	name        string
	endpoint    string
	id          core.SchemaURI
	description string
	attributes  core.Attributes
	extensions  []extension
	repository  Repository[T]
	wrap        func(Service[T]) Service[T]
	validators  []Validator[T]
}

type extension struct {
	id         core.SchemaURI
	attributes core.Attributes
}

func NewResource[T core.Resource](name, endpoint string, id core.SchemaURI, attributes ...*core.Attribute) *Resource[T] {
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

// WithExtension adds a schema extension to the resource type, per RFC 7643, Section 6.
func (c *Resource[T]) WithExtension(id core.SchemaURI, attributes ...*core.Attribute) *Resource[T] {
	c.extensions = append(c.extensions, extension{id: id, attributes: attributes})
	return c
}

func (c *Resource[T]) WithRepository(repository Repository[T]) *Resource[T] {
	c.repository = repository
	return c
}

func (c *Resource[T]) WithService(wrap func(Service[T]) Service[T]) *Resource[T] {
	c.wrap = wrap
	return c
}

func (c *Resource[T]) WithValidators(validators ...Validator[T]) *Resource[T] {
	c.validators = append(c.validators, validators...)
	return c
}

func (c *Resource[T]) schema(base string) *core.Schema {
	return core.NewSchema(c.id).
		WithName(core.ResourceTypeName(c.name)).
		WithDescription(c.description).
		WithLocation(base + "/Schemas/" + string(c.id)).
		With(c.attributes...)
}

func (c *Resource[T]) resourceType() *core.ResourceType {
	resourceType := &core.ResourceType{
		Schemas:     []core.SchemaURI{core.SchemaResourceType},
		ID:          core.ResourceTypeName(c.name),
		Name:        core.ResourceTypeName(c.name),
		Description: c.description,
		Endpoint:    c.endpoint,
		Schema:      c.id,
	}
	for _, extension := range c.extensions {
		resourceType.Extend(core.SchemaExtension{Schema: extension.id})
	}
	return resourceType
}

func (c *Resource[T]) schemas(base string) core.Schemas {
	schemas := make(core.Schemas, 1, 1+len(c.extensions))
	schemas[0] = c.schema(base)
	for _, extension := range c.extensions {
		schemas = append(schemas, core.NewSchema(extension.id).
			WithLocation(base+"/Schemas/"+string(extension.id)).
			With(extension.attributes...))
	}
	return schemas
}

func (c *Resource[T]) mount(s *Server, schemas core.Schemas) {
	path := s.basePath + c.endpoint
	repository := c.repository
	if repository == nil {
		repository = NewRepository[T](s.base()+c.endpoint, schemas)
	}
	service := NewService(repository, schemas, s.limits, c.validators...)
	if c.wrap != nil {
		service = c.wrap(service)
	}
	controller := NewController(service, schemas, s.limits, s.config)

	s.mux.HandleFunc("GET "+path, s.handle(controller.List))
	s.mux.HandleFunc("POST "+path, s.handle(controller.Create))
	s.mux.HandleFunc("GET "+path+"/{id}", s.handle(controller.ByID))
	s.mux.HandleFunc("PUT "+path+"/{id}", s.handle(controller.Replace))
	s.mux.HandleFunc("PATCH "+path+"/{id}", s.handle(controller.Patch))
	s.mux.HandleFunc("DELETE "+path+"/{id}", s.handle(controller.Delete))
	s.mux.HandleFunc("POST "+path+"/.search", s.handle(controller.Search))
}
