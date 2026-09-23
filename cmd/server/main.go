package main

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/server"
)

func main() {
	basePath := "/scim/v2"
	logError := func(err error) { log.Printf("%v\n", err) }
	errorHandler := func(r *http.Request, err error) {
		method, path := strconv.Quote(r.Method), strconv.Quote(r.URL.Path)
		log.Printf("%s %s: %v\n", method, path, err)
	}
	token := os.Getenv("SCIM_BEARER_TOKEN")
	if token == "" {
		log.Fatal("SCIM_BEARER_TOKEN must be set")
	}

	config := core.NewServiceProviderConfig(basePath).
		Sorting().
		Filtering(protocol.DefaultLimits.MaxCount).
		Patching().
		Versioning()

	srv := server.New(config,
		server.ErrorHandler(errorHandler),
		server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, newUserFields())),
		server.WithResource(server.NewResource[*core.Group]("Group", "/Groups", core.SchemaGroup, newGroupFields())),
		server.WithAuthentication(core.NewOAuthBearerToken().AsPrimary(), server.RequireBearerToken(
			func(ctx context.Context, candidate string) (context.Context, error) {
				if subtle.ConstantTimeCompare([]byte(candidate), []byte(token)) != 1 {
					return ctx, fmt.Errorf("invalid token")
				}
				return ctx, nil
			},
		)),
	)

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
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logError(err)
	}
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
		server.NewElements(
			core.NewMultiValuedAttribute("members", "User", "Group"),
			func(g *core.Group) []core.Member { return g.Members },
			server.NewField(core.NewAttribute("value", core.TypeString), func(m core.Member) any { return m.Value }),
			server.NewField(core.NewAttribute("$ref", core.TypeReference), func(m core.Member) any { return m.Ref }),
			server.NewField(core.NewAttribute("type", core.TypeString).Suggesting("User", "Group"), func(m core.Member) any { return m.Type }),
			server.NewField(core.NewAttribute("display", core.TypeString), func(m core.Member) any { return m.Display }),
		),
	)
}
