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
		server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, core.UserAttributes()...)),
		server.WithResource(server.NewResource[*core.Group]("Group", "/Groups", core.SchemaGroup, core.GroupAttributes()...)),
		server.WithAuthentication(core.NewOAuthBearerToken().AsPrimary(), server.RequireBearerToken(
			func(ctx context.Context, candidate string) (context.Context, error) {
				if subtle.ConstantTimeCompare([]byte(candidate), []byte(token)) != 1 {
					return ctx, server.ErrInvalidToken
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
