package main

import (
	"context"
	"crypto/subtle"
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
	token := os.Getenv("SCIM_BEARER_TOKEN")
	if token == "" {
		log.Fatal("SCIM_BEARER_TOKEN must be set")
	}

	srv := server.New(basePath).
		WithResource(server.
			NewResource[*core.User]("User", "/Users", core.SchemaUser, newUserFields()).
			WithDescription("User Account").
			WithErrorHandler(errorHandler)).
		WithResource(server.
			NewResource[*core.Group]("Group", "/Groups", core.SchemaGroup, newGroupFields()).
			WithDescription("Group").
			WithErrorHandler(errorHandler)).
		WithErrorHandler(errorHandler).
		WithAuthentication(core.NewOAuthBearerToken().AsPrimary(), server.RequireBearerToken(
			func(ctx context.Context, candidate string) (context.Context, error) {
				if subtle.ConstantTimeCompare([]byte(candidate), []byte(token)) != 1 {
					return ctx, fmt.Errorf("invalid token")
				}
				return ctx, nil
			},
		))

	addr := ":8080"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           srv,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

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

func newUserFields() server.Fields[*core.User] {
	return server.NewFields(
		server.NewField(
			core.NewAttribute("userName", core.TypeString).AsRequired().UniqueOn(core.UniquenessServer),
			func(u *core.User) any { return u.UserName },
		),
		server.NewField[*core.User](core.NewAttribute("name", core.TypeComplex), nil).With(
			server.NewField(core.NewAttribute("givenName", core.TypeString), func(u *core.User) any { return u.Name.GivenName }),
			server.NewField(core.NewAttribute("familyName", core.TypeString), func(u *core.User) any { return u.Name.FamilyName }),
		),
		server.NewField(
			core.NewAttribute("active", core.TypeBoolean),
			func(u *core.User) any {
				if u.Active == nil {
					return nil
				}
				return *u.Active
			},
		),
		server.NewMultiValued("emails", func(u *core.User) []core.Email { return u.Emails }, "work", "home", "other"),
	)
}

func newGroupFields() server.Fields[*core.Group] {
	return server.NewFields(
		server.NewField(
			core.NewAttribute("displayName", core.TypeString).AsRequired(),
			func(g *core.Group) any { return g.DisplayName },
		),
	)
}
