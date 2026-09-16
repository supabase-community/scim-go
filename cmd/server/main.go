package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/server"
)

func newUserSchema(basePath string) *core.Schema {
	return (&core.Schema{
		Schemas: []core.SchemaURI{core.SchemaSchema},
		ID:      core.SchemaUser,
		Name:    "User",
		Meta: core.Meta{
			ResourceType: "Schema",
			Location:     basePath + "/Schemas/" + string(core.SchemaUser),
		},
	}).Describe("User Account").With(
		core.NewAttribute("userName", core.TypeString).AsRequired().UniqueOn(core.UniquenessServer),
		core.NewAttribute("name", core.TypeComplex).With(
			core.NewAttribute("givenName", core.TypeString),
			core.NewAttribute("familyName", core.TypeString),
		),
		core.NewAttribute("active", core.TypeBoolean),
		core.NewAttribute("emails", core.TypeComplex).AsMultiValued().With(
			core.NewAttribute("value", core.TypeString),
			core.NewAttribute("type", core.TypeString).Suggesting("work", "home", "other"),
			core.NewAttribute("primary", core.TypeBoolean),
		),
	)
}

func main() {
	basePath := "/scim/v2"
	schema := newUserSchema(basePath)
	users := server.NewResource[*core.User]("User", "/Users", schema)

	srv, err := server.New(basePath, users)
	if err != nil {
		log.Fatal(err)
	}

	httpServer := &http.Server{Addr: ":0", Handler: srv}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	fmt.Println("scim server listening under", basePath)

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
}
