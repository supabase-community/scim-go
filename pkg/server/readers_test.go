package server_test

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

func TestFilterValuesAreTypedByTheirAttribute(t *testing.T) {
	srv := newTestServer(t)
	createWidget(t, srv, &widget{Name: "bolt", Score: 7, When: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)})
	active := true
	create(t, srv, &core.User{UserName: "bjensen", Active: &active, EnterpriseUser: &core.EnterpriseUser{Department: "Tour"}})

	for endpoint, filters := range map[string][]string{
		"/Widgets": {`score gt 5`, `when gt "2026-01-01T00:00:00Z"`, `meta.created pr`, `meta.resourceType eq "Widget"`},
		"/Users":   {`active eq true`, string(core.SchemaEnterpriseUser) + `:department eq "tour"`},
	} {
		for _, filter := range filters {
			t.Run(filter, func(t *testing.T) {
				path := basePath + endpoint + "?" + url.Values{"filter": {filter}}.Encode()
				response := Response(t, srv, Request(t, srv, http.MethodGet, path, WithBearerToken(validToken)))

				require.Equal(t, http.StatusOK, response.StatusCode)
				assert.Equal(t, 1, ReadBodyAs[protocol.ListResponse[map[string]any]](t, response).TotalResults)
			})
		}
	}
}
