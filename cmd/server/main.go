// Experimental: Development Server for testing SCIM 2.0
package main

import (
	"log"
	"net/http"
	"time"

	"github.com/supabase-community/scim-go/internal/scim"
	"github.com/supabase-community/scim-go/pkg/core"
)

func main() {
	const externalURL = "http://example.com"
	baseURL := scim.Join(externalURL, scim.BasePath)

	userSchema := scim.NewUserSchema(baseURL)
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

	users := scim.NewMemoryUserRepository(baseURL)

	srv := scim.NewServer(
		externalURL,
		users,
		[]*core.Schema{userSchema},
		[]*core.ResourceType{userType},
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

	mux.Handle("/", adapt(srv.NotFound))

	const addr = ":8080"
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	log.Printf("scim: listening on %s", addr)
	log.Fatal(httpServer.ListenAndServe())
}

func adapt(h func(w http.ResponseWriter, r *http.Request) error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			log.Printf("scim: %v", err)
		}
	})
}
