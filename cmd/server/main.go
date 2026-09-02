package main

import (
	"log"
	"net/http"

	"github.com/supabase-community/go-scim/internal/scim"
	"github.com/supabase-community/go-scim/internal/server"
	"github.com/supabase-community/go-scim/pkg/core"
)

func main() {
	const externalURL = "http://example.com"
	baseURL := scim.Join(externalURL, scim.BasePath)

	userSchema := server.NewUserSchema(baseURL)
	groupSchema := server.NewGroupSchema(baseURL)
	userType := scim.NewResourceType(baseURL, scim.KindUser, userSchema)
	groupType := scim.NewResourceType(baseURL, scim.KindGroup, groupSchema)

	users := server.NewMemoryUserRepository(baseURL)
	groups := server.NewMemoryGroupRepository(baseURL)

	srv := scim.NewServer(
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
