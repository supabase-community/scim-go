package server

import (
	"errors"
	"net/http"
	"net/url"

	"github.com/supabase-community/go-scim/internal/scim"
	"github.com/supabase-community/go-scim/pkg/core"
	"github.com/supabase-community/go-scim/pkg/protocol"
)

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
