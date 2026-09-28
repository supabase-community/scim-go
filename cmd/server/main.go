package main

import (
	"context"
	"crypto/subtle"
	"errors"
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

const basePath = "/scim/v2"

func main() {
	addr, token := configFromEnv()
	config := core.NewServiceProviderConfig().Sorting().Filtering(protocol.DefaultLimits.MaxCount).Patching().Versioning()
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           newServer(config, token),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Println("scim server listening on", addr, "under", basePath)
	run(ctx.Done(), httpServer)
}

func configFromEnv() (addr, token string) {
	addr = ":8080"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}
	token = os.Getenv("SCIM_BEARER_TOKEN")
	if token == "" {
		log.Fatal("SCIM_BEARER_TOKEN must be set")
	}
	return addr, token
}

// run serves httpServer until done is closed, then shuts it down within a grace period.
func run(done <-chan struct{}, httpServer *http.Server) {
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	<-done
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Printf("%v\n", err)
	}
}

func newServer(config *core.ServiceProviderConfig, token string) http.Handler {
	errorHandler := func(r *http.Request, err error) {
		log.Printf("%s %s: %v\n", strconv.Quote(r.Method), strconv.Quote(r.URL.Path), err)
	}
	return server.New(basePath, config,
		server.ErrorHandler(errorHandler),
		server.WithResource(server.
			NewResource[*core.User]("User", "/Users", core.SchemaUser, core.UserAttributes()...).
			WithExtension(core.SchemaEnterpriseUser, core.EnterpriseUserAttributes()...),
		),
		server.WithResource(server.
			NewResource[*core.Group]("Group", "/Groups", core.SchemaGroup, core.GroupAttributes()...),
		),
		server.WithAuthentication(core.NewOAuthBearerToken().AsPrimary(), server.RequireBearerToken(
			func(ctx context.Context, candidate string) (context.Context, error) {
				if subtle.ConstantTimeCompare([]byte(candidate), []byte(token)) != 1 {
					return ctx, server.ErrInvalidToken
				}
				return ctx, nil
			},
		)),
	)
}
