package server_test

import (
	"math"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/server"
)

func TestAccessorValuesAreNormalized(t *testing.T) {
	fields := server.NewFields(
		server.NewField(core.NewAttribute("userName", core.TypeString), func(u *core.User) any { return u.UserName }),
		server.NewField(core.NewAttribute("length", core.TypeInteger), func(u *core.User) any { return len(u.UserName) }),
		server.NewField(core.NewAttribute("active", core.TypeBoolean), func(u *core.User) any { return u.Active }),
		server.NewField(core.NewAttribute("size", core.TypeInteger), func(u *core.User) any { return uint(len(u.UserName)) }),
		server.NewField(core.NewAttribute("huge", core.TypeDecimal), func(u *core.User) any { return uint64(math.MaxUint64) }),
		server.NewField(core.NewAttribute("half", core.TypeDecimal), func(u *core.User) any { return float32(len(u.UserName)) / 2 }),
	)
	srv := Server(t, server.New(fullServiceProviderConfig(),
		server.WithResource(server.NewResource("User", "/Users", core.SchemaUser, fields)),
	))
	active := true
	response := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Users", WithRequestBodyAs(t, core.User{UserName: "bjensen", Active: &active})))
	require.Equal(t, http.StatusCreated, response.StatusCode)

	for _, filter := range []string{`length gt 5`, `active eq true`, `size eq 7`, `huge gt 1.0`, `half eq 3.5`, `meta.resourceType eq "User"`} {
		t.Run(filter, func(t *testing.T) {
			response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users?filter="+url.QueryEscape(filter)))

			require.Equal(t, http.StatusOK, response.StatusCode)
			assert.Equal(t, 1, ReadBodyAs[protocol.ListResponse[map[string]any]](t, response).TotalResults)
		})
	}
}
