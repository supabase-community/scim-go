package server_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/server"
)

func ExampleNew() {
	handler := server.New("/scim/v2", core.NewServiceProviderConfig(),
		server.WithResource(server.NewResource[*core.User](
			"User", "/Users", core.SchemaUser, core.UserAttributes()...,
		)),
		server.WithAuthentication(core.NewOAuthBearerToken().AsPrimary(),
			server.RequireBearerToken(func(ctx context.Context, token string) (context.Context, error) {
				if token != "secret" {
					return ctx, server.ErrInvalidToken
				}
				return ctx, nil
			}),
		),
	)

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/scim/v2/Users", nil)
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	fmt.Println(response.Code)
	// Output: 200
}

func ExampleHooks() {
	hooks := server.Hooks[*core.User]{
		After: func(_ context.Context, event server.Event[*core.User]) error {
			fmt.Println(event.Op)
			return nil
		},
	}
	handler := server.New("/scim/v2", core.NewServiceProviderConfig(),
		server.WithResource(server.
			NewResource[*core.User]("User", "/Users", core.SchemaUser, core.UserAttributes()...).
			WithService(hooks.Wrap),
		),
	)

	body := strings.NewReader(`{"userName":"bjensen"}`)
	request := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/scim/v2/Users", body)
	request.Header.Set("Content-Type", "application/scim+json")
	handler.ServeHTTP(httptest.NewRecorder(), request)
	// Output: create
}
