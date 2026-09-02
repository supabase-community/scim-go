// Experimental: Development Server for testing SCIM 2.0
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"

	"github.com/gofrs/uuid"
	"github.com/supabase-community/go-scim/pkg/core"
	"github.com/supabase-community/go-scim/pkg/protocol"
	"github.com/supabase-community/go-scim/pkg/scim"
)

func main() {
	const externalURL = "http://example.com"
	baseURL := scim.Join(externalURL, scim.BasePath)

	userSchema := NewUserSchema(baseURL)
	groupSchema := NewGroupSchema(baseURL)
	userType := &core.ResourceType{
		Schemas:     []core.SchemaURI{core.SchemaResourceType},
		ID:          "User",
		Name:        "User",
		Description: userSchema.Description,
		Endpoint:    "/Users",
		Schema:      userSchema.ID,
		Meta: core.Meta{
			ResourceType: "User",
			Location:     scim.Join(baseURL, "/Users"),
		},
	}

	groupType := &core.ResourceType{
		Schemas:     []core.SchemaURI{core.SchemaResourceType},
		ID:          "Group",
		Name:        "Group",
		Description: groupSchema.Description,
		Endpoint:    "/Groups",
		Schema:      groupSchema.ID,
		Meta: core.Meta{
			ResourceType: "Group",
			Location:     scim.Join(baseURL, "/Groups"),
		},
	}

	users := NewMemoryUserRepository(baseURL)
	groups := NewMemoryGroupRepository(baseURL)

	srv := NewServer(
		externalURL,
		users,
		groups,
		[]*core.Schema{userSchema, groupSchema},
		[]*core.ResourceType{userType, groupType},
	)

	mux := http.NewServeMux()

	mux.Handle("GET "+scim.BasePath+"/ServiceProviderConfig", adapt(srv.ServiceProviderConfig))

	mux.Handle("GET "+scim.BasePath+"/ResourceTypes", adapt(srv.ResourceTypes))
	mux.Handle("GET "+scim.BasePath+"/ResourceTypes/{id}", adapt(srv.ResourceTypeByID))

	mux.Handle("GET "+scim.BasePath+"/Schemas", adapt(srv.Schemas))
	mux.Handle("GET "+scim.BasePath+"/Schemas/{id}", adapt(srv.SchemaByID))

	mux.Handle("GET "+scim.BasePath+"/Users", adapt(srv.Users))
	mux.Handle("GET "+scim.BasePath+"/Users/{id}", adapt(srv.UserByID))
	mux.Handle("POST "+scim.BasePath+"/Users", adapt(srv.CreateUser))
	mux.Handle("PUT "+scim.BasePath+"/Users/{id}", adapt(srv.ReplaceUser))
	mux.Handle("DELETE "+scim.BasePath+"/Users/{id}", adapt(srv.DeleteUser))

	mux.Handle("GET "+scim.BasePath+"/Groups", adapt(srv.Groups))
	mux.Handle("GET "+scim.BasePath+"/Groups/{id}", adapt(srv.GroupByID))
	mux.Handle("POST "+scim.BasePath+"/Groups", adapt(srv.CreateGroup))
	mux.Handle("PUT "+scim.BasePath+"/Groups/{id}", adapt(srv.ReplaceGroup))
	mux.Handle("DELETE "+scim.BasePath+"/Groups/{id}", adapt(srv.DeleteGroup))

	mux.Handle("/", adapt(srv.NotFound))

	const addr = ":8080"
	log.Printf("scim: listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func adapt(h func(w http.ResponseWriter, r *http.Request) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			log.Printf("scim: %v", err)
		}
	})
}

func validateUser(user *core.User) *protocol.Error {
	if user.UserName == "" {
		return protocol.ErrInvalidValue(`"userName" is required`)
	}
	if !slices.Contains(user.Schemas, core.SchemaUser) {
		return protocol.ErrInvalidValue(`"schemas" must include the User schema URN`)
	}
	return nil
}

func decodeUser(r *http.Request) (*core.User, error) {
	body, err := readBody(r)
	if err != nil {
		return nil, err
	}

	user := new(core.User)
	if err := json.Unmarshal(body, user); err != nil {
		return nil, protocol.ErrInvalidSyntax("request body is not a valid User")
	}
	return user, nil
}

func validateGroup(group *core.Group) *protocol.Error {
	if group.DisplayName == "" {
		return protocol.ErrInvalidValue(`"displayName" is required`)
	}
	if !slices.Contains(group.Schemas, core.SchemaGroup) {
		return protocol.ErrInvalidValue(`"schemas" must include the Group schema URN`)
	}
	return nil
}

func decodeGroup(r *http.Request) (*core.Group, error) {
	body, err := readBody(r)
	if err != nil {
		return nil, err
	}

	group := new(core.Group)
	if err := json.Unmarshal(body, group); err != nil {
		return nil, protocol.ErrInvalidSyntax("request body is not a valid Group")
	}
	return group, nil
}

func readBody(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return nil, protocol.ErrTooLarge("the request body is too large")
		}
		return nil, protocol.ErrInvalidSyntax("could not read the request body")
	}
	return body, nil
}

type UserRepository struct {
	mu      sync.RWMutex
	baseURL string
	items   map[string]*core.User
}

func NewMemoryUserRepository(baseURL string) scim.Repository[*core.User] {
	return &UserRepository{baseURL: baseURL, items: make(map[string]*core.User)}
}

func (r *UserRepository) Get(ctx context.Context, id string) (*core.User, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return item, nil
}

func (r *UserRepository) List(ctx context.Context, query *protocol.SearchRequest) ([]*core.User, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	all := make([]*core.User, 0, len(r.items))
	for _, item := range r.items {
		all = append(all, item)
	}
	slices.SortFunc(all, func(a, b *core.User) int { return strings.Compare(a.ID, b.ID) })

	total := len(all)
	start := min(query.Offset(), total)
	end := min(start+query.Count, total)
	return all[start:end], total, nil
}

func (r *UserRepository) Create(ctx context.Context, user *core.User) (*core.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	user.ID = uuid.Must(uuid.NewV4()).String()
	r.items[user.ID] = user
	return user, nil
}

func (r *UserRepository) Replace(ctx context.Context, id string, user *core.User) (*core.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.items[id]; !ok {
		return nil, ErrNotFound
	}
	user.ID = id
	r.items[id] = user
	return user, nil
}

func (r *UserRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}

var ErrNotFound = errors.New("scim: resource not found")

type Server struct {
	serviceProviderConfig *core.ServiceProviderConfig
	resourceTypes         []*core.ResourceType
	schemas               []*core.Schema
	users                 resourceServer[*core.User]
	groups                resourceServer[*core.Group]
}

func NewServer(
	externalURL string,
	users scim.Repository[*core.User],
	groups scim.Repository[*core.Group],
	schemas []*core.Schema,
	resourceTypes []*core.ResourceType,
) *Server {
	baseURL := scim.Join(externalURL, scim.BasePath)

	limits := protocol.DefaultLimits
	return &Server{
		serviceProviderConfig: scim.NewServiceProviderConfig(
			baseURL,
			core.NewOAuthBearerToken().AsPrimary(),
		).Sorting(),
		resourceTypes: resourceTypes,
		schemas:       schemas,
		users: resourceServer[*core.User]{
			limits: limits, repo: users, validate: validateUser, decode: decodeUser,
		},
		groups: resourceServer[*core.Group]{
			limits: limits, repo: groups, validate: validateGroup, decode: decodeGroup,
		},
	}
}

func (srv *Server) ServiceProviderConfig(w http.ResponseWriter, r *http.Request) error {
	return protocol.Send(w, http.StatusOK, srv.serviceProviderConfig)
}

func (srv *Server) ResourceTypes(w http.ResponseWriter, r *http.Request) error {
	return listCollection(w, r, srv.resourceTypes)
}

func (srv *Server) ResourceTypeByID(w http.ResponseWriter, r *http.Request) error {
	return findByID(w, r, srv.resourceTypes)
}

func (srv *Server) Schemas(w http.ResponseWriter, r *http.Request) error {
	return listCollection(w, r, srv.schemas)
}

func (srv *Server) SchemaByID(w http.ResponseWriter, r *http.Request) error {
	return findByID(w, r, srv.schemas)
}

func (srv *Server) Users(w http.ResponseWriter, r *http.Request) error { return srv.users.list(w, r) }
func (srv *Server) UserByID(w http.ResponseWriter, r *http.Request) error {
	return srv.users.byID(w, r)
}
func (srv *Server) CreateUser(w http.ResponseWriter, r *http.Request) error {
	return srv.users.create(w, r)
}
func (srv *Server) ReplaceUser(w http.ResponseWriter, r *http.Request) error {
	return srv.users.replace(w, r)
}
func (srv *Server) DeleteUser(w http.ResponseWriter, r *http.Request) error {
	return srv.users.delete(w, r)
}

func (srv *Server) Groups(w http.ResponseWriter, r *http.Request) error { return srv.groups.list(w, r) }
func (srv *Server) GroupByID(w http.ResponseWriter, r *http.Request) error {
	return srv.groups.byID(w, r)
}
func (srv *Server) CreateGroup(w http.ResponseWriter, r *http.Request) error {
	return srv.groups.create(w, r)
}
func (srv *Server) ReplaceGroup(w http.ResponseWriter, r *http.Request) error {
	return srv.groups.replace(w, r)
}
func (srv *Server) DeleteGroup(w http.ResponseWriter, r *http.Request) error {
	return srv.groups.delete(w, r)
}

func (srv *Server) NotFound(w http.ResponseWriter, r *http.Request) error {
	return notFound(w)
}

func listCollection[T any](w http.ResponseWriter, r *http.Request, resources []T) error {
	if rejected, err := rejectFilter(w, r); rejected {
		return err
	}
	return protocol.Send(w, http.StatusOK, protocol.NewListResponse(1, len(resources), resources))
}

func findByID[T core.Resource](w http.ResponseWriter, r *http.Request, resources []T) error {
	id := urlParam(r, "id")
	for _, resource := range resources {
		if resource.ResourceID() == id {
			return protocol.Send(w, http.StatusOK, resource)
		}
	}
	return notFound(w)
}

type resourceServer[T core.Resource] struct {
	limits   protocol.Limits
	repo     scim.Repository[T]
	validate func(T) *protocol.Error
	decode   func(*http.Request) (T, error)
}

func (s *resourceServer[T]) list(w http.ResponseWriter, r *http.Request) error {
	query, err := s.limits.ParseSearchRequest(r.URL.Query())
	if err != nil {
		return protocol.SendError(w, err)
	}

	items, total, err := s.repo.List(r.Context(), query)
	if err != nil {
		return protocol.SendError(w, err)
	}

	return protocol.Send(w, http.StatusOK, protocol.NewListResponse(query.StartIndex, total, items))
}

func (s *resourceServer[T]) byID(w http.ResponseWriter, r *http.Request) error {
	item, err := s.repo.Get(r.Context(), urlParam(r, "id"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return notFound(w)
		}
		return protocol.SendError(w, err)
	}
	return protocol.Send(w, http.StatusOK, item)
}

func (s *resourceServer[T]) create(w http.ResponseWriter, r *http.Request) error {
	item, err := s.decode(r)
	if err != nil {
		return protocol.SendError(w, err)
	}
	if err := s.validate(item); err != nil {
		return protocol.SendError(w, err)
	}

	created, err := s.repo.Create(r.Context(), item)
	if err != nil {
		return protocol.SendError(w, err)
	}

	w.Header().Set("Location", locationOf(created))
	return protocol.Send(w, http.StatusCreated, created)
}

func (s *resourceServer[T]) replace(w http.ResponseWriter, r *http.Request) error {
	id := urlParam(r, "id")

	item, err := s.decode(r)
	if err != nil {
		return protocol.SendError(w, err)
	}
	if err := s.validate(item); err != nil {
		return protocol.SendError(w, err)
	}

	replaced, err := s.repo.Replace(r.Context(), id, item)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return notFound(w)
		}
		return protocol.SendError(w, err)
	}
	return protocol.Send(w, http.StatusOK, replaced)
}

func (s *resourceServer[T]) delete(w http.ResponseWriter, r *http.Request) error {
	if err := s.repo.Delete(r.Context(), urlParam(r, "id")); err != nil {
		if errors.Is(err, ErrNotFound) {
			return notFound(w)
		}
		return protocol.SendError(w, err)
	}
	return protocol.Send(w, http.StatusNoContent, nil)
}

type located interface{ Location() string }

func locationOf(v any) string {
	if l, ok := v.(located); ok {
		return l.Location()
	}
	return ""
}

func notFound(w http.ResponseWriter) error {
	return protocol.SendError(w, protocol.ErrNotFound("Endpoint or resource does not exist"))
}

func rejectFilter(w http.ResponseWriter, r *http.Request) (bool, error) {
	if !r.URL.Query().Has("filter") {
		return false, nil
	}
	return true, protocol.SendError(w, protocol.ErrForbidden("Filtering is not supported on this endpoint"))
}

func urlParam(r *http.Request, key string) string {
	if decoded, err := url.PathUnescape(r.PathValue(key)); err == nil {
		return decoded
	}
	return r.PathValue(key)
}

func NewUserSchema(baseURL string) *core.Schema {
	schema := &core.Schema{
		Schemas: []core.SchemaURI{core.SchemaSchema},
		ID:      core.SchemaUser,
		Name:    "User",
		Meta: core.Meta{
			ResourceType: "User",
			Location:     scim.Join(baseURL, "/Users"),
		},
	}

	return schema.
		Describe("User Account").
		With(
			core.NewAttribute("userName", core.TypeString, "Unique identifier for the User").
				AsRequired().
				UniqueOn(core.UniquenessServer),
			core.NewAttribute("name", core.TypeComplex, "The components of the user's name.").
				With(
					core.NewAttribute("formatted", core.TypeString, "The name formatted for display."),
					core.NewAttribute("familyName", core.TypeString, "The family name of the User."),
					core.NewAttribute("givenName", core.TypeString, "The given name of the User."),
					core.NewAttribute("middleName", core.TypeString, "The middle name(s) of the User."),
				),
			core.NewAttribute("emails", core.TypeComplex, "Email addresses for the user.").
				AsMultiValued().
				With(
					core.NewAttribute("value", core.TypeString, "An email address for the user."),
					core.NewAttribute("primary", core.TypeBoolean, "The 'primary' email address"),
				),
			core.NewAttribute("active", core.TypeBoolean, ""),
		)
}

func NewGroupSchema(baseURL string) *core.Schema {
	schema := &core.Schema{
		Schemas: []core.SchemaURI{core.SchemaSchema},
		ID:      core.SchemaGroup,
		Name:    "Group",
		Meta: core.Meta{
			ResourceType: "Group",
			Location:     scim.Join(baseURL, "/Groups"),
		},
	}
	return schema.
		Describe("Group").
		With(
			core.NewAttribute("displayName", core.TypeString, "A human-readable name for the Group.").
				AsRequired(),
			core.NewAttribute("members", core.TypeComplex, "A list of members of the Group.").
				AsMultiValued().
				With(
					core.NewAttribute("value", core.TypeString, "The identifier of a member of this Group.").AsImmutable(),
					core.NewAttribute("$ref", core.TypeReference, "The URI of the User or Group that is a member of this Group.").AsImmutable().Referencing("User", "Group"),
					core.NewAttribute("type", core.TypeString, "A label indicating the type of resource, e.g. 'User' or 'Group'.").AsImmutable().Suggesting("User", "Group"),
					core.NewAttribute("display", core.TypeString, "A human-readable name for the member.").AsImmutable(),
				),
		)
}

type memoryGroupRepository struct {
	mu      sync.RWMutex
	baseURL string
	items   map[string]*core.Group
}

func NewMemoryGroupRepository(baseURL string) scim.Repository[*core.Group] {
	return &memoryGroupRepository{baseURL: baseURL, items: make(map[string]*core.Group)}
}

func (r *memoryGroupRepository) Get(ctx context.Context, id string) (*core.Group, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	item, ok := r.items[id]
	if !ok {
		return nil, ErrNotFound
	}
	return item, nil
}

func (r *memoryGroupRepository) List(ctx context.Context, query *protocol.SearchRequest) ([]*core.Group, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	all := make([]*core.Group, 0, len(r.items))
	for _, item := range r.items {
		all = append(all, item)
	}
	slices.SortFunc(all, func(a, b *core.Group) int { return strings.Compare(a.ID, b.ID) })

	total := len(all)
	start := min(query.Offset(), total)
	end := min(start+query.Count, total)
	return all[start:end], total, nil
}

func (r *memoryGroupRepository) Create(ctx context.Context, group *core.Group) (*core.Group, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	group.ID = uuid.Must(uuid.NewV4()).String()
	r.items[group.ID] = group
	return group, nil
}

func (r *memoryGroupRepository) Replace(ctx context.Context, id string, group *core.Group) (*core.Group, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.items[id]; !ok {
		return nil, ErrNotFound
	}
	group.ID = id
	r.items[id] = group
	return group, nil
}

func (r *memoryGroupRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}
	delete(r.items, id)
	return nil
}
