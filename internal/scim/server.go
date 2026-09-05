package scim

import (
	"log"
	"net/http"
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

// BasePath is the mount point of the SCIM service, per RFC 7644, Section 3.2.
const BasePath = "/scim/v2"

func Join(base, segment string) string {
	return strings.TrimSuffix(base, "/") + "/" + strings.TrimPrefix(segment, "/")
}

func NewServiceProviderConfig(baseURL string, schemes ...*core.AuthenticationScheme) *core.ServiceProviderConfig {
	if schemes == nil {
		schemes = []*core.AuthenticationScheme{}
	}
	return &core.ServiceProviderConfig{
		Schemas:               []core.SchemaURI{core.SchemaServiceProviderConfig},
		AuthenticationSchemes: schemes,
		Meta: core.Meta{
			ResourceType: "ServiceProviderConfig",
			Location:     Join(baseURL, "/ServiceProviderConfig"),
		},
	}
}

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
		users:         newResourceServer(limits, users, validateUser, decodeUser),
	}
}

// RegisterRoutes mounts every SCIM endpoint served by srv onto mux.
func (srv *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("GET "+BasePath+"/ServiceProviderConfig", adapt(srv.ServiceProviderConfig))

	mux.Handle("GET "+BasePath+"/ResourceTypes", adapt(srv.ResourceTypes))
	mux.Handle("GET "+BasePath+"/ResourceTypes/{id}", adapt(srv.ResourceTypeByID))

	mux.Handle("GET "+BasePath+"/Schemas", adapt(srv.Schemas))
	mux.Handle("GET "+BasePath+"/Schemas/{id}", adapt(srv.SchemaByID))

	mux.Handle("GET "+BasePath+"/Users", adapt(srv.Users))
	mux.Handle("GET "+BasePath+"/Users/{id}", adapt(srv.UserByID))
	mux.Handle("POST "+BasePath+"/Users", adapt(srv.CreateUser))
	mux.Handle("PUT "+BasePath+"/Users/{id}", adapt(srv.ReplaceUser))
	mux.Handle("DELETE "+BasePath+"/Users/{id}", adapt(srv.DeleteUser))

	mux.Handle("/", adapt(srv.NotFound))
}

func adapt(h func(w http.ResponseWriter, r *http.Request) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			log.Printf("scim: %v", err)
		}
	})
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
