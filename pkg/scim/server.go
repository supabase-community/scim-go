package scim

import (
	"net/http"

	"github.com/supabase-community/go-scim/pkg/core"
	"github.com/supabase-community/go-scim/pkg/protocol"
)

// BasePath is the mount point of the SCIM service, per RFC 7644, Section 3.2.
const BasePath = "/scim/v2"

type Server struct {
	serviceProviderConfig *core.ServiceProviderConfig
	resourceTypes         []*core.ResourceType
	schemas               []*core.Schema
	users                 resourceServer[*core.User]
	groups                resourceServer[*core.Group]
}

func NewServer(
	externalURL string,
	users Repository[*core.User],
	groups Repository[*core.Group],
	schemas []*core.Schema,
	resourceTypes []*core.ResourceType,
) *Server {
	baseURL := core.Join(externalURL, BasePath)

	limits := protocol.DefaultLimits
	return &Server{
		serviceProviderConfig: core.NewServiceProviderConfig(
			baseURL,
			core.NewOAuthBearerToken().AsPrimary(),
		).Sorting().Filtering(limits.MaxCount),
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
