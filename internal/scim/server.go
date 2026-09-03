package scim

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"uuid"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

func validateUser(user *core.User) *protocol.Error {
	if user.UserName == "" {
		return protocol.ErrInvalidValue(`"userName" is required`)
	}
	if !slices.Contains(user.Schemas, core.SchemaUser) {
		return protocol.ErrInvalidValue(`"schemas" must include the User schema URN`)
	}
	return nil
}

func readBody(r *http.Request) ([]byte, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return nil, protocol.ErrTooLarge("the request body is too large")
		}
		return nil, protocol.ErrInvalidSyntax("could not read the request body")
	}
	return body, nil
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

type UserRepository struct {
	mu      sync.RWMutex
	baseURL string
	items   map[string]*core.User
}

func NewMemoryUserRepository(baseURL string) Repository[*core.User] {
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

	user.ID = uuid.NewV7().String()
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
}

func NewServer(
	externalURL string,
	users Repository[*core.User],
	schemas []*core.Schema,
	resourceTypes []*core.ResourceType,
) *Server {
	baseURL := Join(externalURL, BasePath)

	limits := protocol.DefaultLimits
	return &Server{
		serviceProviderConfig: NewServiceProviderConfig(
			baseURL,
			core.NewOAuthBearerToken().AsPrimary(),
		).Sorting(),
		resourceTypes: resourceTypes,
		schemas:       schemas,
		users: resourceServer[*core.User]{
			limits: limits, repo: users, validate: validateUser, decode: decodeUser,
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
	repo     Repository[T]
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
			Location:     Join(baseURL, "/Users"),
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
