package main

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/server"
)

const basePath = "/scim/v2"

var logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

func main() {
	addr, token, baseURL := configFromEnv()
	config := core.NewServiceProviderConfig().Sorting().Filtering(protocol.DefaultLimits.MaxCount).Patching().Versioning()
	httpServer := &http.Server{
		Addr:              addr,
		Handler:           newServer(config, token, baseURL),
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

func configFromEnv() (addr, token, baseURL string) {
	addr = ":8080"
	if port := os.Getenv("PORT"); port != "" {
		addr = ":" + port
	}
	token = os.Getenv("SCIM_BEARER_TOKEN")
	if token == "" {
		logger.Error("SCIM_BEARER_TOKEN must be set")
		os.Exit(1)
	}
	return addr, token, os.Getenv("SCIM_BASE_URL")
}

func run(done <-chan struct{}, httpServer *http.Server) {
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("listen failed", "error", err)
			os.Exit(1)
		}
	}()

	<-done
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logger.ErrorContext(shutdownCtx, "shutdown failed", "error", err)
	}
}

func newServer(config *core.ServiceProviderConfig, token, baseURL string) http.Handler {
	errorHandler := func(r *http.Request, err error) {
		logger.ErrorContext(r.Context(), "request failed", "method", r.Method, "path", r.URL.Path, "error", err)
	}
	return server.New(basePath, config,
		server.ErrorHandler(errorHandler),
		server.WithBaseURL(baseURL),
		server.WithResource(server.
			NewResource[*core.User]("User", "/Users", core.SchemaUser, core.UserAttributes()...).
			WithExtension(core.SchemaEnterpriseUser, core.EnterpriseUserAttributes()...).
			WithService(logged[*core.User]()),
		),
		server.WithResource(server.
			NewResource[*core.Group]("Group", "/Groups", core.SchemaGroup, core.GroupAttributes()...).
			WithService(logged[*core.Group]()),
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

func logAt[T core.Resource](message string) func(context.Context, server.Event[T]) error {
	return func(ctx context.Context, event server.Event[T]) error {
		logger.InfoContext(ctx, message, "event", event)
		return nil
	}
}

func logged[T core.Resource]() func(server.Service[T]) server.Service[T] {
	return server.Hooks[T]{Before: logAt[T]("before"), After: logAt[T]("after")}.Wrap
}
