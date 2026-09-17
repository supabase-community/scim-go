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

func main() {
	basePath := "/scim/v2"
	errorHandler := func(err error) { log.Printf("%v\n", err) }
	srv, err := server.New(
		basePath,
		server.
			NewResource[*core.User]("User", "/Users", newUserSchema(basePath), newUserGetters()).
			WithErrorHandler(errorHandler),
	)
	if err != nil {
		log.Fatal(err)
	}

	addr := ":8080"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}
	httpServer := &http.Server{Addr: addr, Handler: srv}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	fmt.Println("scim server listening on", addr, "under", basePath)

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdownCtx)
}

func newUserSchema(basePath string) *core.Schema {
	return core.NewSchema(core.SchemaUser).
		WithName("User").
		WithLocation(basePath+"/Schemas/"+string(core.SchemaUser)).
		WithDescription("User Account").With(
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

func newUserGetters() server.Getters[*core.User] {
	return server.Getters[*core.User]{
		"username": func(u *core.User) any { return u.UserName },
		"active": func(u *core.User) any {
			if u.Active == nil {
				return nil
			}
			return *u.Active
		},
		"name.givenname":  func(u *core.User) any { return u.Name.GivenName },
		"name.familyname": func(u *core.User) any { return u.Name.FamilyName },
		"emails.value": func(u *core.User) any {
			values := make([]any, len(u.Emails))
			for i, e := range u.Emails {
				values[i] = e.Value
			}
			return values
		},
		"emails.type": func(u *core.User) any {
			values := make([]any, len(u.Emails))
			for i, e := range u.Emails {
				values[i] = e.Type
			}
			return values
		},
		"emails.primary": func(u *core.User) any {
			values := make([]any, len(u.Emails))
			for i, e := range u.Emails {
				values[i] = e.Primary != nil && *e.Primary
			}
			return values
		},
	}
}
