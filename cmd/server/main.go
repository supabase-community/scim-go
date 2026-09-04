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

	srv := scim.NewServer(
		externalURL,
		scim.NewMemoryUserRepository(baseURL),
		[]*core.Schema{userSchema},
		[]*core.ResourceType{userType},
	)

	mux := http.NewServeMux()
	srv.RegisterRoutes(mux)

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
