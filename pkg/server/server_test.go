package server_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
	"github.com/supabase-community/scim-go/pkg/server"
)

const basePath = "/scim/v2"
const validToken = "s3cr3t"

func TestRFC6750(t *testing.T) {
	t.Run("2.1 Authorization Request Header Field", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodGet, basePath+"/Users",
			WithContentType(protocol.MediaType),
			WithHeader("Authorization", "Bearer "+validToken),
		)
		response := Response(t, srv, request)

		assert.Equal(t, http.StatusOK, response.StatusCode)
	})

	// Also RFC 7644 2 Authentication and Authorization (WWW-Authenticate SHALL, per RFC 7235 4.1)
	t.Run("3 The WWW-Authenticate Response Header Field", func(t *testing.T) {
		t.Run("omits error info when no Authorization header is present", func(t *testing.T) {
			srv := newTestServer(t)

			request := Request(t, srv, http.MethodGet, basePath+"/Users", WithContentType(protocol.MediaType))
			response := Response(t, srv, request)

			assert.Equal(t, http.StatusUnauthorized, response.StatusCode)
			assert.Equal(t, "Bearer", response.Header.Get("WWW-Authenticate"))
		})

		t.Run("omits error info for a non-Bearer scheme", func(t *testing.T) {
			srv := newTestServer(t)

			request := Request(t, srv, http.MethodGet, basePath+"/Users",
				WithContentType(protocol.MediaType),
				WithHeader("Authorization", "Basic dXNlcjpwYXNz"),
			)
			response := Response(t, srv, request)

			assert.Equal(t, http.StatusUnauthorized, response.StatusCode)
			assert.Equal(t, "Bearer", response.Header.Get("WWW-Authenticate"))
		})
	})

	t.Run("3.1 Error Codes", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodGet, basePath+"/Users",
			WithContentType(protocol.MediaType),
			WithHeader("Authorization", "Bearer wrong"),
		)
		response := Response(t, srv, request)

		assert.Equal(t, http.StatusUnauthorized, response.StatusCode)
		assert.Contains(t, response.Header.Get("WWW-Authenticate"), `error="invalid_token"`)
	})
}

// 2.2 Anonymous Requests
func TestRFC7644AnonymousRequests(t *testing.T) {
	srv := newTestServer(t)

	request := Request(t, srv, http.MethodPost, basePath+"/Users", WithContentType(protocol.MediaType))
	response := Response(t, srv, request)

	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
}

// 3.3 Creating Resources
// Also RFC 7643 2.2 Attribute Characteristics (required, uniqueness, canonical values) and 3.1 Common Attributes (id, meta)
func TestRFC7644CreatingResources(t *testing.T) {
	t.Run("creates a resource and returns 201 with Location and ETag", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodPost, basePath+"/Users",
			WithAcceptHeader(protocol.MediaType),
			WithContentType(protocol.MediaType),
			WithBearerToken(validToken),
			WithRequestBodyAs(t, core.User{UserName: "bjensen"}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusCreated, response.StatusCode)
		assert.Equal(t, protocol.MediaType, response.Header.Get("Content-Type"))
		user := ReadBodyAs[*core.User](t, response)
		assert.Equal(t, basePath+"/Users/"+user.ID, response.Header.Get("Location"))
		assert.NotEmpty(t, response.Header.Get("ETag"))
		assert.Equal(t, []core.SchemaURI{core.SchemaUser}, user.Schemas)
		assert.NotEmpty(t, user.ID)
		assert.Equal(t, "bjensen", user.UserName)
		assert.Equal(t, core.ResourceTypeName("User"), user.Meta.ResourceType)
		assert.NotZero(t, user.Meta.Created)
		assert.NotZero(t, user.Meta.LastModified)
		assert.NotEmpty(t, user.Meta.Location)
		assert.NotEmpty(t, user.Meta.Version)
	})

	t.Run("rejects a malformed JSON body", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodPost, basePath+"/Users", WithBearerToken(validToken), WithRequestBody([]byte(`{`)))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		scimErr := ReadBodyAs[scimerrors.Error](t, response)
		assert.Equal(t, scimerrors.InvalidSyntax, scimErr.ScimType)
	})

	t.Run("rejects a resource missing a required attribute", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodPost, basePath+"/Users",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, core.User{}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidValue, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	t.Run("rejects an element missing a required sub-attribute", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodPost, basePath+"/Widgets",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, &widget{Name: "gear", Parts: []part{{Serial: "s-1"}, {}}}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidValue, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	t.Run("rejects a value outside the declared canonical values", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodPost, basePath+"/Users",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, core.User{UserName: "bjensen", UserType: "bogus"}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidValue, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	t.Run("rejects an element value outside the declared canonical values", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodPost, basePath+"/Users",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, core.User{UserName: "bjensen", Emails: []core.Email{
				{Value: "b@work.com", Type: "work"},
				{Value: "b@elsewhere.com", Type: "bogus"},
			}}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidValue, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	t.Run("rejects a duplicate value for a unique attribute", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "bjensen"})

		request := Request(t, srv, http.MethodPost, basePath+"/Users",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, core.User{UserName: "bjensen"}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusConflict, response.StatusCode)
		assert.Equal(t, scimerrors.Uniqueness, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	t.Run("skips a candidate whose optional unique attribute is unset", func(t *testing.T) {
		srv := newTestServer(t)
		createWidget(t, srv, &widget{Name: "a"})
		createWidget(t, srv, &widget{Name: "b"})
	})

	t.Run("rejects a JSON null body instead of panicking", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodPost, basePath+"/Users",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBody([]byte(`null`)),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidSyntax, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	t.Run("a case-exact unique attribute treats different casing as distinct", func(t *testing.T) {
		srv := newTestServer(t)
		createWidget(t, srv, &widget{Name: "a", Nick: "Al"})
		createWidget(t, srv, &widget{Name: "b", Nick: "al"})

		request := Request(t, srv, http.MethodPost, basePath+"/Widgets",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, &widget{Name: "c", Nick: "Al"}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusConflict, response.StatusCode)
		assert.Equal(t, scimerrors.Uniqueness, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})
}

// 3.4.1 Retrieving a Known Resource
// Also RFC 7643 3.1 Common Attributes
func TestRFC7644RetrievingAKnownResource(t *testing.T) {
	t.Run("gets a created resource by id", func(t *testing.T) {
		srv := newTestServer(t)
		id, etag := create(t, srv, &core.User{UserName: "bjensen"})

		request := Request(t, srv, http.MethodGet, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, etag, response.Header.Get("ETag"))
		assert.Equal(t, "bjensen", ReadBodyAs[core.User](t, response).UserName)
	})

	t.Run("returns 404 for an unknown id", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodGet, basePath+"/Users/does-not-exist",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
		)
		response := Response(t, srv, request)

		assert.Equal(t, http.StatusNotFound, response.StatusCode)
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		assert.JSONEq(t, `{
			"schemas": ["urn:ietf:params:scim:api:messages:2.0:Error"],
			"status": "404",
			"detail": "Not found"
		}`, string(body))
	})
}

// 3.4.2 Query Resources
// Also RFC 7644 3.9 Additional Operation Response Parameters (totalResults, itemsPerPage)
func TestRFC7644QueryResources(t *testing.T) {
	t.Run("lists all created resources", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice"})
		create(t, srv, &core.User{UserName: "bob"})
		create(t, srv, &core.User{UserName: "carol"})

		request := Request(t, srv, http.MethodGet, basePath+"/Users",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		assert.Equal(t, 3, list.TotalResults)
		assert.Equal(t, "alice", list.Resources[0].UserName)
		assert.Equal(t, []core.SchemaURI{core.SchemaUser}, list.Resources[0].Schemas)
		assert.NotEmpty(t, list.Resources[0].Meta.Location)
		assert.Equal(t, "bob", list.Resources[1].UserName)
		assert.Equal(t, "carol", list.Resources[2].UserName)
	})

	t.Run("paginates with startIndex and count", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice"})
		create(t, srv, &core.User{UserName: "bob"})
		create(t, srv, &core.User{UserName: "carol"})

		path := basePath + "/Users?" + url.Values{"startIndex": {"2"}, "count": {"1"}}.Encode()
		request := Request(t, srv, http.MethodGet, path,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		assert.Equal(t, 3, list.TotalResults)
		assert.Equal(t, 1, list.ItemsPerPage)
		require.Len(t, list.Resources, 1)
		assert.Equal(t, "bob", list.Resources[0].UserName)
	})

	t.Run("returns an empty list when there are no resources", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodGet, basePath+"/Users",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		assert.Equal(t, 0, list.TotalResults)
		assert.Empty(t, list.Resources)
	})

	t.Run("rejects a query with a non-integer startIndex", func(t *testing.T) {
		srv := newTestServer(t)

		path := basePath + "/Users?" + url.Values{"startIndex": {"bogus"}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidValue, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})
}

// 3.4.2.2 Filtering
func TestRFC7644Filtering(t *testing.T) {
	t.Run("filters resources by an exact match on userName", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice"})
		create(t, srv, &core.User{UserName: "bob"})

		path := basePath + "/Users?" + url.Values{"filter": {`userName eq "alice"`}}.Encode()
		request := Request(t, srv, http.MethodGet, path,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Equal(t, 1, list.TotalResults)
		assert.Equal(t, "alice", list.Resources[0].UserName)
	})

	t.Run("rejects a filter on an attribute that is not filterable", func(t *testing.T) {
		srv := newTestServer(t)

		path := basePath + "/Users?" + url.Values{"filter": {`bogus eq "x"`}}.Encode()
		request := Request(t, srv, http.MethodGet, path,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidFilter, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	t.Run("filters on a boolean attribute with eq and ne", func(t *testing.T) {
		srv := newTestServer(t)
		active := true
		inactive := false
		create(t, srv, &core.User{UserName: "alice", Active: &active})
		create(t, srv, &core.User{UserName: "bob", Active: &inactive})

		path := basePath + "/Users?" + url.Values{"filter": {`active eq true`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Equal(t, 1, list.TotalResults)
		assert.Equal(t, "alice", list.Resources[0].UserName)

		path = basePath + "/Users?" + url.Values{"filter": {`active ne true`}}.Encode()
		request = Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response = Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list = ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Equal(t, 1, list.TotalResults)
		assert.Equal(t, "bob", list.Resources[0].UserName)
	})

	t.Run("filters with the pr operator for presence", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice"})

		path := basePath + "/Users?" + url.Values{"filter": {`userName pr`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Equal(t, 1, list.TotalResults)
	})

	t.Run("pr matches a non-string value and excludes an absent one", func(t *testing.T) {
		srv := newTestServer(t)
		active := true
		create(t, srv, &core.User{UserName: "alice", Active: &active})
		create(t, srv, &core.User{UserName: "bob"})

		path := basePath + "/Users?" + url.Values{"filter": {`active pr`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Equal(t, 1, list.TotalResults)
		assert.Equal(t, "alice", list.Resources[0].UserName)
	})

	t.Run("filters by eq and pr on the common id attribute", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "alice"})
		create(t, srv, &core.User{UserName: "bob"})

		path := basePath + "/Users?" + url.Values{"filter": {`id eq "` + id + `"`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Equal(t, 1, list.TotalResults)
		assert.Equal(t, "alice", list.Resources[0].UserName)

		path = basePath + "/Users?" + url.Values{"filter": {`id pr`}}.Encode()
		request = Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response = Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list = ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		assert.Equal(t, 2, list.TotalResults)
	})

	t.Run("rejects pr on a complex attribute that has no accessor of its own", func(t *testing.T) {
		srv := newTestServer(t)

		path := basePath + "/Users?" + url.Values{"filter": {`name pr`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidFilter, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	t.Run("rejects a value path filter whose inner expression is invalid", func(t *testing.T) {
		srv := newTestServer(t)

		path := basePath + "/Users?" + url.Values{"filter": {`emails[bogus eq "x"]`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidFilter, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	t.Run("filters by presence on the other common attributes", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice", ExternalID: "ext-1"})
		create(t, srv, &core.User{UserName: "bob"})

		cases := map[string]int{
			`externalId pr`:        1,
			`meta.resourceType pr`: 2,
			`meta.created pr`:      2,
			`meta.lastModified pr`: 2,
			`meta.version pr`:      2,
			`meta.location pr`:     2,
		}
		for filterExpr, want := range cases {
			path := basePath + "/Users?" + url.Values{"filter": {filterExpr}}.Encode()
			request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
			response := Response(t, srv, request)

			require.Equal(t, http.StatusOK, response.StatusCode, "filter: %s", filterExpr)
			assert.Equal(t, want, ReadBodyAs[protocol.ListResponse[*core.User]](t, response).TotalResults, "filter: %s", filterExpr)
		}
	})

	t.Run("filters with the co, sw, and ew string operators", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice"})
		create(t, srv, &core.User{UserName: "bob"})

		cases := []string{`userName co "li"`, `userName sw "al"`, `userName ew "ce"`}
		for _, filter := range cases {
			path := basePath + "/Users?" + url.Values{"filter": {filter}}.Encode()
			request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
			response := Response(t, srv, request)

			require.Equal(t, http.StatusOK, response.StatusCode, "filter: %s", filter)
			list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
			require.Equal(t, 1, list.TotalResults, "filter: %s", filter)
			assert.Equal(t, "alice", list.Resources[0].UserName, "filter: %s", filter)
		}
	})

	t.Run("combines clauses with and", func(t *testing.T) {
		srv := newTestServer(t)
		active := true
		create(t, srv, &core.User{UserName: "alice", Active: &active})
		create(t, srv, &core.User{UserName: "bob", Active: &active})

		path := basePath + "/Users?" + url.Values{"filter": {`userName eq "alice" and active eq true`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Equal(t, 1, list.TotalResults)
		assert.Equal(t, "alice", list.Resources[0].UserName)
	})

	t.Run("combines clauses with or", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice"})
		create(t, srv, &core.User{UserName: "bob"})
		create(t, srv, &core.User{UserName: "carol"})

		path := basePath + "/Users?" + url.Values{"filter": {`userName eq "alice" or userName eq "carol"`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Equal(t, 2, list.TotalResults)
		assert.Equal(t, "alice", list.Resources[0].UserName)
		assert.Equal(t, "carol", list.Resources[1].UserName)
	})

	t.Run("negates a clause with not", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice"})
		create(t, srv, &core.User{UserName: "carol"})

		path := basePath + "/Users?" + url.Values{"filter": {`not (userName eq "carol")`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Equal(t, 1, list.TotalResults)
		assert.Equal(t, "alice", list.Resources[0].UserName)
	})

	t.Run("filters with the ordering operators ne, gt, ge, lt, and le", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice"})
		create(t, srv, &core.User{UserName: "bob"})
		create(t, srv, &core.User{UserName: "carol"})

		cases := map[string]string{
			`userName ne "bob"`: "alice,carol",
			`userName gt "bob"`: "carol",
			`userName ge "bob"`: "bob,carol",
			`userName lt "bob"`: "alice",
			`userName le "bob"`: "alice,bob",
		}
		for filter, want := range cases {
			path := basePath + "/Users?" + url.Values{"filter": {filter}, "sortBy": {"userName"}}.Encode()
			request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
			response := Response(t, srv, request)

			require.Equal(t, http.StatusOK, response.StatusCode, "filter: %s", filter)
			list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
			names := make([]string, len(list.Resources))
			for i, u := range list.Resources {
				names[i] = u.UserName
			}
			assert.Equal(t, strings.Split(want, ","), names, "filter: %s", filter)
		}
	})

	// 3.4.2.2 Value Filters
	t.Run("filters using a value path expression on a multi-valued complex attribute", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice", Emails: []core.Email{
			{Value: "a@work.com", Type: "work"},
			{Value: "a@home.com", Type: "home"},
		}})
		create(t, srv, &core.User{UserName: "bob", Emails: []core.Email{
			{Value: "b@home.com", Type: "home"},
		}})

		path := basePath + "/Users?" + url.Values{"filter": {`emails[type eq "work"]`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Equal(t, 1, list.TotalResults)
		assert.Equal(t, "alice", list.Resources[0].UserName)
	})

	t.Run("a value path filter with and requires both conditions on the same element", func(t *testing.T) {
		srv := newTestServer(t)
		alicePrimary := false
		aliceHome := true
		bobPrimary := true
		create(t, srv, &core.User{UserName: "alice", Emails: []core.Email{
			{Value: "a@work.com", Type: "work", Primary: &alicePrimary},
			{Value: "a@home.com", Type: "home", Primary: &aliceHome},
		}})
		create(t, srv, &core.User{UserName: "bob", Emails: []core.Email{
			{Value: "b@work.com", Type: "work", Primary: &bobPrimary},
		}})

		path := basePath + "/Users?" + url.Values{"filter": {`emails[type eq "work" and primary eq true]`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Equal(t, 1, list.TotalResults)
		assert.Equal(t, "bob", list.Resources[0].UserName)
	})

	t.Run("a value path filter combines conditions with or", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice", Emails: []core.Email{{Value: "a@home.com", Type: "home"}}})
		create(t, srv, &core.User{UserName: "bob", Emails: []core.Email{{Value: "b@other.com", Type: "other"}}})

		path := basePath + "/Users?" + url.Values{"filter": {`emails[type eq "work" or type eq "home"]`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Equal(t, 1, list.TotalResults)
		assert.Equal(t, "alice", list.Resources[0].UserName)
	})

	t.Run("a value path filter negates a condition with not", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice", Emails: []core.Email{{Value: "a@home.com", Type: "home"}}})
		create(t, srv, &core.User{UserName: "bob", Emails: []core.Email{{Value: "b@work.com", Type: "work"}}})

		path := basePath + "/Users?" + url.Values{"filter": {`emails[not (type eq "home")]`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Equal(t, 1, list.TotalResults)
		assert.Equal(t, "bob", list.Resources[0].UserName)
	})

	t.Run("a value path filter tests presence within the element", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice", Emails: []core.Email{{Value: "a@home.com"}}})
		create(t, srv, &core.User{UserName: "bob", Emails: []core.Email{{Value: "b@work.com", Type: "work"}}})

		path := basePath + "/Users?" + url.Values{"filter": {`emails[type pr]`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Equal(t, 1, list.TotalResults)
		assert.Equal(t, "bob", list.Resources[0].UserName)
	})

	// RFC 7644 3.4.2.2 - dot notation matches any element; brackets match the same element.
	t.Run("dot notation matches conditions on different elements but a value path does not", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice", Emails: []core.Email{
			{Value: "a@work.com", Type: "work", Primary: new(false)},
			{Value: "a@home.com", Type: "home", Primary: new(true)},
		}})

		cases := map[string]int{
			`emails.type eq "work" and emails.primary eq true`: 1,
			`emails[type eq "work" and primary eq true]`:       0,
		}
		for filter, want := range cases {
			path := basePath + "/Users?" + url.Values{"filter": {filter}}.Encode()
			request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
			response := Response(t, srv, request)

			require.Equal(t, http.StatusOK, response.StatusCode, "filter: %s", filter)
			list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
			assert.Equal(t, want, list.TotalResults, "filter: %s", filter)
		}
	})

	t.Run("filters an integer attribute with the ordering operators", func(t *testing.T) {
		srv := newTestServer(t)
		createWidget(t, srv, &widget{Name: "low", Score: 5})
		createWidget(t, srv, &widget{Name: "high", Score: 50})

		cases := map[string]string{
			`score eq 5`:  "low",
			`score ne 5`:  "high",
			`score gt 10`: "high",
			`score ge 50`: "high",
			`score lt 10`: "low",
			`score le 5`:  "low",
		}
		for filter, want := range cases {
			path := basePath + "/Widgets?" + url.Values{"filter": {filter}}.Encode()
			request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
			response := Response(t, srv, request)

			require.Equal(t, http.StatusOK, response.StatusCode, "filter: %s", filter)
			list := ReadBodyAs[protocol.ListResponse[map[string]any]](t, response)
			require.Equal(t, 1, list.TotalResults, "filter: %s", filter)
			assert.Equal(t, want, list.Resources[0]["Name"], "filter: %s", filter)
		}
	})

	t.Run("filters a dateTime attribute with the ordering operators", func(t *testing.T) {
		srv := newTestServer(t)
		early := time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC)
		late := time.Date(2021, 6, 1, 0, 0, 0, 0, time.UTC)
		createWidget(t, srv, &widget{Name: "early", When: early})
		createWidget(t, srv, &widget{Name: "late", When: late})

		cases := map[string]string{
			`when eq "2020-06-01T00:00:00Z"`: "early",
			`when ne "2020-06-01T00:00:00Z"`: "late",
			`when gt "2020-12-31T00:00:00Z"`: "late",
			`when ge "2021-06-01T00:00:00Z"`: "late",
			`when lt "2020-12-31T00:00:00Z"`: "early",
			`when le "2020-06-01T00:00:00Z"`: "early",
		}
		for filter, want := range cases {
			path := basePath + "/Widgets?" + url.Values{"filter": {filter}}.Encode()
			request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
			response := Response(t, srv, request)

			require.Equal(t, http.StatusOK, response.StatusCode, "filter: %s", filter)
			list := ReadBodyAs[protocol.ListResponse[map[string]any]](t, response)
			require.Equal(t, 1, list.TotalResults, "filter: %s", filter)
			assert.Equal(t, want, list.Resources[0]["Name"], "filter: %s", filter)
		}
	})

	// Also RFC 7643 2.4 Multi-Valued Attributes
	t.Run("filters a multi-valued simple attribute", func(t *testing.T) {
		srv := newTestServer(t)
		createWidget(t, srv, &widget{Name: "tagged", Tags: []any{"red", "blue"}})
		createWidget(t, srv, &widget{Name: "untagged", Tags: []any{"green"}})

		path := basePath + "/Widgets?" + url.Values{"filter": {`tags eq "red"`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[map[string]any]](t, response)
		require.Equal(t, 1, list.TotalResults)
		assert.Equal(t, "tagged", list.Resources[0]["Name"])
	})

	t.Run("a candidate missing the filtered attribute does not match", func(t *testing.T) {
		srv := newTestServer(t)
		createWidget(t, srv, &widget{Name: "has-nick", Nick: "al"})
		createWidget(t, srv, &widget{Name: "no-nick"})

		path := basePath + "/Widgets?" + url.Values{"filter": {`nick eq "al"`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[map[string]any]](t, response)
		require.Equal(t, 1, list.TotalResults)
		assert.Equal(t, "has-nick", list.Resources[0]["Name"])
	})
}

// 3.4.2.3 Sorting
// Also RFC 7644 3.10 Attribute Notation (dot notation for sub-attributes)
func TestRFC7644Sorting(t *testing.T) {
	t.Run("sorts ascending by default when only sortBy is given", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice"})
		create(t, srv, &core.User{UserName: "bob"})
		create(t, srv, &core.User{UserName: "carol"})

		path := basePath + "/Users?" + url.Values{"sortBy": {"userName"}}.Encode()
		request := Request(t, srv, http.MethodGet, path,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Len(t, list.Resources, 3)
		assert.Equal(t, "alice", list.Resources[0].UserName)
		assert.Equal(t, "bob", list.Resources[1].UserName)
		assert.Equal(t, "carol", list.Resources[2].UserName)
	})

	t.Run("sorts descending when sortOrder is descending", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice"})
		create(t, srv, &core.User{UserName: "bob"})
		create(t, srv, &core.User{UserName: "carol"})

		path := basePath + "/Users?" + url.Values{"sortBy": {"userName"}, "sortOrder": {"descending"}}.Encode()
		request := Request(t, srv, http.MethodGet, path,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Len(t, list.Resources, 3)
		assert.Equal(t, "carol", list.Resources[0].UserName)
		assert.Equal(t, "bob", list.Resources[1].UserName)
		assert.Equal(t, "alice", list.Resources[2].UserName)
	})

	t.Run("sorts by the common id attribute", func(t *testing.T) {
		srv := newTestServer(t)
		first, _ := create(t, srv, &core.User{UserName: "alice"})
		second, _ := create(t, srv, &core.User{UserName: "bob"})
		want := []string{first, second}
		slices.Sort(want)

		path := basePath + "/Users?" + url.Values{"sortBy": {"id"}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Len(t, list.Resources, 2)
		assert.Equal(t, want[0], list.Resources[0].ID)
		assert.Equal(t, want[1], list.Resources[1].ID)
	})

	t.Run("sorts by a nested sub-attribute", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "u1", Name: core.Name{GivenName: "Zoe"}})
		create(t, srv, &core.User{UserName: "u2", Name: core.Name{GivenName: "Amy"}})

		path := basePath + "/Users?" + url.Values{"sortBy": {"name.givenName"}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Len(t, list.Resources, 2)
		assert.Equal(t, "u2", list.Resources[0].UserName)
		assert.Equal(t, "u1", list.Resources[1].UserName)
	})

	t.Run("resources both missing the sort attribute keep their relative order", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "first"})
		create(t, srv, &core.User{UserName: "second"})

		path := basePath + "/Users?" + url.Values{"sortBy": {"name.givenName"}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Len(t, list.Resources, 2)
		assert.Equal(t, "first", list.Resources[0].UserName)
		assert.Equal(t, "second", list.Resources[1].UserName)
	})

	// RFC 7644 Section 3.4.2.3: resources without a value are ordered last if ascending and first if descending.
	t.Run("resources missing the sort attribute sort first when descending", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "no-name"})
		create(t, srv, &core.User{UserName: "has-name", Name: core.Name{GivenName: "Amy"}})

		path := basePath + "/Users?" + url.Values{"sortBy": {"name.givenName"}, "sortOrder": {"descending"}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Len(t, list.Resources, 2)
		assert.Equal(t, "no-name", list.Resources[0].UserName)
		assert.Equal(t, "has-name", list.Resources[1].UserName)
	})

	t.Run("rejects sortBy on an attribute that is not known", func(t *testing.T) {
		srv := newTestServer(t)

		path := basePath + "/Users?" + url.Values{"sortBy": {"bogus"}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidValue, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	t.Run("rejects sortBy with an invalid attribute path syntax", func(t *testing.T) {
		srv := newTestServer(t)

		path := basePath + "/Users?" + url.Values{"sortBy": {"1bad"}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidValue, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	t.Run("sorts by a boolean attribute, with false ranking before true", func(t *testing.T) {
		srv := newTestServer(t)
		active := true
		inactive := false
		create(t, srv, &core.User{UserName: "alice", Active: &active})
		create(t, srv, &core.User{UserName: "bob", Active: &inactive})
		create(t, srv, &core.User{UserName: "carol"})

		path := basePath + "/Users?" + url.Values{"sortBy": {"active"}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Len(t, list.Resources, 3)
		assert.Equal(t, "bob", list.Resources[0].UserName)
		assert.Equal(t, "alice", list.Resources[1].UserName)
		assert.Equal(t, "carol", list.Resources[2].UserName)

		path = basePath + "/Users?" + url.Values{"sortBy": {"active"}, "sortOrder": {"descending"}}.Encode()
		request = Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response = Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list = ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Len(t, list.Resources, 3)
		assert.Equal(t, "carol", list.Resources[0].UserName)
		assert.Equal(t, "alice", list.Resources[1].UserName)
		assert.Equal(t, "bob", list.Resources[2].UserName)
	})

	t.Run("sorts by an integer attribute", func(t *testing.T) {
		srv := newTestServer(t)
		createWidget(t, srv, &widget{Name: "high", Score: 50})
		createWidget(t, srv, &widget{Name: "low", Score: 5})

		path := basePath + "/Widgets?" + url.Values{"sortBy": {"score"}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[map[string]any]](t, response)
		require.Len(t, list.Resources, 2)
		assert.Equal(t, "low", list.Resources[0]["Name"])
		assert.Equal(t, "high", list.Resources[1]["Name"])
	})

	t.Run("sorts by a dateTime attribute", func(t *testing.T) {
		srv := newTestServer(t)
		early := time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC)
		late := time.Date(2021, 6, 1, 0, 0, 0, 0, time.UTC)
		createWidget(t, srv, &widget{Name: "early", When: early})
		createWidget(t, srv, &widget{Name: "late", When: late})

		path := basePath + "/Widgets?" + url.Values{"sortBy": {"when"}, "sortOrder": {"descending"}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[map[string]any]](t, response)
		require.Len(t, list.Resources, 2)
		assert.Equal(t, "late", list.Resources[0]["Name"])
		assert.Equal(t, "early", list.Resources[1]["Name"])
	})

	// RFC 7644 Section 3.4.2.3: a multi-valued attribute sorts by its primary value, or else its first value.
	t.Run("sorts by the primary value of a multi-valued attribute", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "zed", Emails: []core.Email{
			{Value: "a@example.com", Primary: new(false)},
			{Value: "z@example.com", Primary: new(true)},
		}})
		create(t, srv, &core.User{UserName: "mia", Emails: []core.Email{
			{Value: "m@example.com"},
			{Value: "b@example.com"},
		}})
		create(t, srv, &core.User{UserName: "nobody"})

		path := basePath + "/Users?" + url.Values{"sortBy": {"emails.value"}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Len(t, list.Resources, 3)
		assert.Equal(t, "mia", list.Resources[0].UserName)
		assert.Equal(t, "zed", list.Resources[1].UserName)
		assert.Equal(t, "nobody", list.Resources[2].UserName)
	})
}

// 3.4.2.5 Attributes
// RFC 7644 Sections 3.4.2.5 and 3.9: attributes and excludedAttributes shape every returned resource.
func TestRFC7644Attributes(t *testing.T) {
	srv := newTestServer(t)
	id, _ := create(t, srv, &core.User{
		UserName:    "bjensen",
		DisplayName: "Babs Jensen",
		Emails:      []core.Email{{Value: "bjensen@example.com", Type: "work"}},
	})
	get := func(t *testing.T, path string) (int, map[string]any) {
		t.Helper()
		response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+path, WithBearerToken(validToken)))
		return response.StatusCode, ReadBodyAs[map[string]any](t, response)
	}

	t.Run("lists only the minimum set and the requested attributes", func(t *testing.T) {
		status, body := get(t, "/Users?attributes=userName")
		require.Equal(t, http.StatusOK, status)
		resources := body["Resources"].([]any)
		require.Len(t, resources, 1)
		assert.ElementsMatch(t, []string{"schemas", "id", "userName"}, keysOf(resources[0].(map[string]any)))
	})

	t.Run("fetches a resource without the excluded attributes", func(t *testing.T) {
		status, body := get(t, "/Users/"+id+"?excludedAttributes=emails,meta")
		require.Equal(t, http.StatusOK, status)
		assert.Equal(t, "bjensen", body["userName"])
		assert.NotContains(t, body, "emails")
		assert.NotContains(t, body, "meta")
	})

	t.Run("omits attributes the schema does not declare", func(t *testing.T) {
		_, body := get(t, "/Users/"+id)
		assert.NotContains(t, body, "displayName")
	})

	t.Run("rejects attributes together with excludedAttributes", func(t *testing.T) {
		status, _ := get(t, "/Users/"+id+"?attributes=userName&excludedAttributes=emails")
		assert.Equal(t, http.StatusBadRequest, status)
	})

	t.Run("rejects both parameters on a write before changing anything", func(t *testing.T) {
		query := "?attributes=userName&excludedAttributes=emails"
		for method, path := range map[string]string{
			http.MethodPost:  "/Users" + query,
			http.MethodPut:   "/Users/" + id + query,
			http.MethodPatch: "/Users/" + id + query,
		} {
			response := Response(t, srv, Request(t, srv, method, basePath+path,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBodyAs(t, core.User{UserName: "mallory"}),
			))
			assert.Equal(t, http.StatusBadRequest, response.StatusCode, method)
		}
		_, body := get(t, "/Users?filter=userName%20eq%20%22mallory%22")
		assert.InDelta(t, 0, body["totalResults"], 0)
	})

	t.Run("shapes the resource returned by a create", func(t *testing.T) {
		response := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Users?attributes=userName",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, core.User{UserName: "alice", Emails: []core.Email{{Value: "a@example.com"}}}),
		))
		require.Equal(t, http.StatusCreated, response.StatusCode)
		assert.ElementsMatch(t, []string{"schemas", "id", "userName"}, keysOf(ReadBodyAs[map[string]any](t, response)))
	})

	t.Run("shapes the resource returned by a patch", func(t *testing.T) {
		response := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Users/"+id+"?excludedAttributes=emails",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{{Op: patch.OpReplace, Path: "userType", Value: json.RawMessage(`"employee"`)}},
			}),
		))
		require.Equal(t, http.StatusOK, response.StatusCode)
		body := ReadBodyAs[map[string]any](t, response)
		assert.Equal(t, "employee", body["userType"])
		assert.NotContains(t, body, "emails")
	})
}

func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// 3.4.3 Alternative Query with POST /.search
func TestRFC7644AlternativeQueryWithPOSTSearch(t *testing.T) {
	t.Skip("POST /.search is not registered by server.New; falls through to the generic unknown-path 404")
}

// 3.5.1 Replacing with PUT
// Also RFC 7643 2.2 Attribute Characteristics (mutability)
func TestRFC7644ReplacingWithPUT(t *testing.T) {
	t.Run("replaces a resource and returns a new ETag when If-Match matches", func(t *testing.T) {
		srv := newTestServer(t)
		id, etag := create(t, srv, &core.User{UserName: "bjensen"})

		request := Request(t, srv, http.MethodPut, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithHeader("If-Match", etag),
			WithRequestBody([]byte(`{"userName":"bjensen2"}`)),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.NotEmpty(t, response.Header.Get("ETag"))
		replaced := ReadBodyAs[core.User](t, response)
		assert.Equal(t, "bjensen2", replaced.UserName)
	})

	t.Run("the new ETag differs from the old one even within the same second", func(t *testing.T) {
		srv := newTestServer(t)
		id, etag := create(t, srv, &core.User{UserName: "bjensen"})

		request := Request(t, srv, http.MethodPut, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithHeader("If-Match", etag),
			WithRequestBody([]byte(`{"userName":"bjensen2"}`)),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.NotEqual(t, etag, response.Header.Get("ETag"))
	})

	t.Run("replaces a resource even without If-Match", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		request := Request(t, srv, http.MethodPut, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBody([]byte(`{"userName":"bjensen2"}`)),
		)
		response := Response(t, srv, request)

		assert.Equal(t, http.StatusOK, response.StatusCode)
	})

	t.Run("rejects a replace with a stale If-Match", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		request := Request(t, srv, http.MethodPut, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithHeader("If-Match", `W/"stale"`),
			WithRequestBody([]byte(`{"userName":"bjensen2"}`)),
		)
		response := Response(t, srv, request)

		assert.Equal(t, http.StatusPreconditionFailed, response.StatusCode)
	})

	t.Run("replacing an unknown id returns 404", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodPut, basePath+"/Users/does-not-exist",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBody([]byte(`{"userName":"bjensen"}`)),
		)
		response := Response(t, srv, request)

		assert.Equal(t, http.StatusNotFound, response.StatusCode)
	})

	t.Run("rejects a replace with a malformed JSON body", func(t *testing.T) {
		srv := newTestServer(t)
		id, etag := create(t, srv, &core.User{UserName: "bjensen"})

		request := Request(t, srv, http.MethodPut, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithHeader("If-Match", etag),
			WithRequestBody([]byte(`{not-json`)),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidSyntax, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	t.Run("rejects a replace with a JSON null body instead of panicking", func(t *testing.T) {
		srv := newTestServer(t)
		id, etag := create(t, srv, &core.User{UserName: "bjensen"})

		request := Request(t, srv, http.MethodPut, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithHeader("If-Match", etag),
			WithRequestBody([]byte(`null`)),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidSyntax, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	t.Run("rejects a replace that changes an immutable attribute", func(t *testing.T) {
		srv := newTestServer(t)
		id, etag := create(t, srv, &core.User{UserName: "bjensen", Name: core.Name{FamilyName: "Jensen"}})

		request := Request(t, srv, http.MethodPut, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithHeader("If-Match", etag),
			WithRequestBodyAs(t, core.User{UserName: "bjensen", Name: core.Name{FamilyName: "Smith"}}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.Mutability, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	t.Run("allows assigning an immutable attribute for the first time", func(t *testing.T) {
		srv := newTestServer(t)
		id, etag := create(t, srv, &core.User{UserName: "bjensen"})

		request := Request(t, srv, http.MethodPut, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithHeader("If-Match", etag),
			WithRequestBodyAs(t, core.User{UserName: "bjensen", Name: core.Name{FamilyName: "Jensen"}}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, "Jensen", ReadBodyAs[core.User](t, response).Name.FamilyName)
	})
}

// 3.5.2 Modifying with PATCH
// Also RFC 7643 2.2 Attribute Characteristics (mutability)
func TestRFC7644ModifyingWithPATCH(t *testing.T) {
	t.Run("patches a resource and returns the updated field with an ETag", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		request := Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas: []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{
					{
						Op:    patch.OpReplace,
						Path:  "active",
						Value: json.RawMessage("true"),
					},
				},
			}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.NotEmpty(t, response.Header.Get("ETag"))

		patched := ReadBodyAs[core.User](t, response)
		require.NotNil(t, patched.Active)
		assert.True(t, *patched.Active)
	})

	t.Run("rejects a patch with a malformed JSON body", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		request := Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBody([]byte(`{not-json`)),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidSyntax, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	t.Run("rejects a patch that targets an unknown attribute", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		request := Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas: []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{
					{
						Op:    patch.OpReplace,
						Path:  "bogus",
						Value: json.RawMessage(`"x"`),
					},
				},
			}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidPath, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	t.Run("rejects a patch that changes an immutable attribute", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen", Name: core.Name{FamilyName: "Jensen"}})

		request := Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas: []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{
					{
						Op:    patch.OpReplace,
						Path:  "name.familyName",
						Value: json.RawMessage(`"Smith"`),
					},
				},
			}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.Mutability, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	t.Run("rejects a patch that collides with another resource's unique value", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice"})
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		request := Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas: []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{
					{
						Op:    patch.OpReplace,
						Path:  "userName",
						Value: json.RawMessage(`"alice"`),
					},
				},
			}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusConflict, response.StatusCode)
		assert.Equal(t, scimerrors.Uniqueness, ReadBodyAs[scimerrors.Error](t, response).ScimType)

		// RFC 7644 Section 3.5.2: on error, the original SCIM resource MUST be restored.
		request = Request(t, srv, http.MethodGet, basePath+"/Users/"+id, WithBearerToken(validToken))
		response = Response(t, srv, request)
		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, "bjensen", ReadBodyAs[core.User](t, response).UserName)
	})

	t.Run("patching an unknown id returns 404", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodPatch, basePath+"/Users/does-not-exist",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas: []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{
					{
						Op:    patch.OpReplace,
						Path:  "active",
						Value: json.RawMessage("true"),
					},
				},
			}),
		)
		response := Response(t, srv, request)

		assert.Equal(t, http.StatusNotFound, response.StatusCode)
	})

	t.Run("rejects a patch with a stale If-Match", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		request := Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithHeader("If-Match", `W/"stale"`),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas: []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{
					{
						Op:    patch.OpReplace,
						Path:  "active",
						Value: json.RawMessage("true"),
					},
				},
			}),
		)
		response := Response(t, srv, request)

		assert.Equal(t, http.StatusPreconditionFailed, response.StatusCode)
	})

	t.Run("adds a value with an add operation", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		request := Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas: []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{
					{
						Op:    patch.OpAdd,
						Path:  "name.givenName",
						Value: json.RawMessage(`"Barbara"`),
					},
				},
			}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		patched := ReadBodyAs[core.User](t, response)
		assert.Equal(t, "Barbara", patched.Name.GivenName)
	})

	t.Run("removes a value with a remove operation", func(t *testing.T) {
		srv := newTestServer(t)
		active := true
		id, _ := create(t, srv, &core.User{UserName: "bjensen", Active: &active})

		request := Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas: []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{
					{
						Op:   patch.OpRemove,
						Path: "active",
					},
				},
			}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		patched := ReadBodyAs[core.User](t, response)
		assert.Nil(t, patched.Active)
	})
}

// 3.6 Deleting Resources
func TestRFC7644DeletingResources(t *testing.T) {
	t.Run("deletes a resource and it is subsequently gone", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		request := Request(t, srv, http.MethodDelete, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
		)
		response := Response(t, srv, request)
		require.Equal(t, http.StatusNoContent, response.StatusCode)
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		assert.Empty(t, body)

		getResp := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
		))
		assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
	})

	t.Run("deletes a resource when If-Match matches", func(t *testing.T) {
		srv := newTestServer(t)
		id, etag := create(t, srv, &core.User{UserName: "bjensen"})

		request := Request(t, srv, http.MethodDelete, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithHeader("If-Match", etag),
		)
		response := Response(t, srv, request)
		assert.Equal(t, http.StatusNoContent, response.StatusCode)

		getResp := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
		))
		assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
	})

	t.Run("rejects a delete with a stale If-Match and leaves the resource intact", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		request := Request(t, srv, http.MethodDelete, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithHeader("If-Match", `W/"stale"`),
		)
		response := Response(t, srv, request)
		assert.Equal(t, http.StatusPreconditionFailed, response.StatusCode)

		request = Request(t, srv, http.MethodGet, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
		)
		response = Response(t, srv, request)
		assert.Equal(t, http.StatusOK, response.StatusCode)
	})

	t.Run("deleting an unknown id returns 404", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodDelete, basePath+"/Users/does-not-exist",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
		)
		response := Response(t, srv, request)
		assert.Equal(t, http.StatusNotFound, response.StatusCode)
	})
}

// 3.7 Bulk Operations
func TestRFC7644BulkOperations(t *testing.T) {
	t.Skip("Bulk is not registered by server.New; falls through to the generic unknown-path 404")
}

// 3.11 Singular Resource /Me
func TestRFC7644SingularResourceMe(t *testing.T) {
	t.Skip("/Me is not implemented by server.New; requests to it 404 through the generic unknown-path behavior")
}

// 3.12 HTTP Status and Error Response Handling
func TestRFC7644SCIMErrors(t *testing.T) {
	srv := newTestServer(t)

	request := Request(t, srv, http.MethodGet, basePath+"/Users/does-not-exist",
		WithBearerToken(validToken),
		WithContentType(protocol.MediaType),
	)
	response := Response(t, srv, request)

	require.Equal(t, http.StatusNotFound, response.StatusCode)
	assert.Equal(t, protocol.MediaType, response.Header.Get("Content-Type"))

	scimErr := ReadBodyAs[scimerrors.Error](t, response)
	assert.Equal(t, []core.SchemaURI{scimerrors.SchemaError}, scimErr.Schemas)
	assert.Equal(t, "404", scimErr.Status)
	assert.Empty(t, scimErr.ScimType)
	assert.NotEmpty(t, scimErr.Detail)
}

// 3.14 Versioning Resources
func TestRFC7644ETags(t *testing.T) {
	t.Run("issues a weak ETag", func(t *testing.T) {
		srv := newTestServer(t)
		_, etag := create(t, srv, &core.User{UserName: "bjensen"})

		assert.True(t, strings.HasPrefix(etag, `W/"`))
	})

	t.Run("meta.version matches the ETag header on create", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodPost, basePath+"/Users",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, core.User{UserName: "bjensen"}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusCreated, response.StatusCode)
		created := ReadBodyAs[core.User](t, response)
		assert.Equal(t, response.Header.Get("ETag"), created.Meta.Version)
	})

	t.Run("meta.version matches the ETag header on replace", func(t *testing.T) {
		srv := newTestServer(t)
		id, etag := create(t, srv, &core.User{UserName: "bjensen"})

		request := Request(t, srv, http.MethodPut, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithHeader("If-Match", etag),
			WithRequestBody([]byte(`{"userName":"bjensen2"}`)),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		replaced := ReadBodyAs[core.User](t, response)
		assert.Equal(t, response.Header.Get("ETag"), replaced.Meta.Version)
	})

	t.Run("advertises etag support in ServiceProviderConfig", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodGet, basePath+"/ServiceProviderConfig",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		config := ReadBodyAs[core.ServiceProviderConfig](t, response)
		assert.True(t, config.ETag.Supported)
	})
}

// 4. Service Provider Configuration Endpoints (/ServiceProviderConfig)
// Also RFC 7643 5 Service Provider Configuration Schema
func TestRFC7644ServiceProviderConfiguration(t *testing.T) {
	srv := newTestServer(t)

	request := Request(t, srv, http.MethodGet, basePath+"/ServiceProviderConfig",
		WithBearerToken(validToken),
		WithContentType(protocol.MediaType),
	)
	response := Response(t, srv, request)

	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, protocol.MediaType, response.Header.Get("Content-Type"))

	config := ReadBodyAs[core.ServiceProviderConfig](t, response)
	assert.True(t, config.Patch.Supported)
	assert.True(t, config.Filter.Supported)
	assert.Equal(t, protocol.DefaultLimits.MaxCount, config.Filter.MaxResults)
	assert.True(t, config.Sort.Supported)

	require.Len(t, config.AuthenticationSchemes, 1)
	assert.Equal(t, core.AuthenticationSchemeOAuthBearerToken, config.AuthenticationSchemes[0].Type)
	assert.True(t, config.AuthenticationSchemes[0].Primary)
}

// 4. Service Provider Configuration Endpoints (/Schemas)
// Also RFC 7643 7 Schema Definition
func TestRFC7644Schemas(t *testing.T) {
	t.Run("lists the registered schemas", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodGet, basePath+"/Schemas",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.Schema]](t, response)
		require.Equal(t, 4, list.TotalResults)
		schema := list.Resources[0]
		assert.Equal(t, core.SchemaUser, schema.ID)
		assert.Equal(t, core.SchemaEnterpriseUser, list.Resources[1].ID)
		assert.Equal(t, core.SchemaGroup, list.Resources[2].ID)
		assert.Equal(t, widgetSchema, list.Resources[3].ID)

		userName := schema.Attributes.Lookup("userName")
		require.NotNil(t, userName)
		assert.True(t, userName.Required)

		name := schema.Attributes.Lookup("name")
		require.NotNil(t, name)
		assert.NotNil(t, name.SubAttributes.Lookup("givenName"))

		emails := schema.Attributes.Lookup("emails")
		require.NotNil(t, emails)
		assert.True(t, emails.MultiValued)
	})

	t.Run("fetches a schema by id", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodGet, basePath+"/Schemas/"+string(core.SchemaUser),
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		schema := ReadBodyAs[core.Schema](t, response)
		assert.Equal(t, core.SchemaUser, schema.ID)
	})

	t.Run("returns 404 for an unknown schema id", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodGet, basePath+"/Schemas/urn:bogus",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
		)
		response := Response(t, srv, request)
		assert.Equal(t, http.StatusNotFound, response.StatusCode)
	})
}

// 4. Service Provider Configuration Endpoints (/ResourceTypes)
// Also RFC 7643 6 ResourceType Schema
func TestRFC7644ResourceTypes(t *testing.T) {
	t.Run("lists the registered resource types", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodGet, basePath+"/ResourceTypes",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.ResourceType]](t, response)
		require.Equal(t, 3, list.TotalResults)
		assert.Equal(t, core.ResourceTypeName("User"), list.Resources[0].ID)
		assert.Equal(t, basePath+"/Users", list.Resources[0].Endpoint)
		assert.Equal(t, core.SchemaUser, list.Resources[0].Schema)
		assert.Equal(t, core.ResourceTypeName("Group"), list.Resources[1].ID)
		assert.Equal(t, basePath+"/Groups", list.Resources[1].Endpoint)
		assert.Equal(t, core.SchemaGroup, list.Resources[1].Schema)
		assert.Equal(t, core.ResourceTypeName("Widget"), list.Resources[2].ID)
		assert.Equal(t, basePath+"/Widgets", list.Resources[2].Endpoint)
		assert.Equal(t, widgetSchema, list.Resources[2].Schema)
	})

	t.Run("fetches a resource type by id", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodGet, basePath+"/ResourceTypes/User",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		resourceType := ReadBodyAs[core.ResourceType](t, response)
		assert.Equal(t, core.ResourceTypeName("User"), resourceType.ID)
		assert.Equal(t, basePath+"/Users", resourceType.Endpoint)
		assert.Equal(t, core.SchemaUser, resourceType.Schema)
	})

	t.Run("returns 404 for an unknown resource type id", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodGet, basePath+"/ResourceTypes/Bogus",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
		)
		response := Response(t, srv, request)

		assert.Equal(t, http.StatusNotFound, response.StatusCode)
	})
}

// RFC 7643 4.3 Enterprise User Schema Extension
func TestRFC7643EnterpriseUserExtension(t *testing.T) {
	srv := newTestServer(t)

	id, _ := create(t, srv, &core.User{
		UserName: "bjensen",
		EnterpriseUser: &core.EnterpriseUser{
			EmployeeNumber: "1234",
			Department:     "Tour Operations",
		},
	})

	request := Request(t, srv, http.MethodGet, basePath+"/Users/"+id,
		WithBearerToken(validToken),
		WithContentType(protocol.MediaType),
	)
	response := Response(t, srv, request)

	require.Equal(t, http.StatusOK, response.StatusCode)
	user := ReadBodyAs[core.User](t, response)
	require.NotNil(t, user.EnterpriseUser)
	assert.Equal(t, "1234", user.EnterpriseUser.EmployeeNumber)
	assert.Equal(t, "Tour Operations", user.EnterpriseUser.Department)

	// RFC 7643 Section 6: a resource type lists the schema extensions it accepts.
	t.Run("advertises the extension on the resource type", func(t *testing.T) {
		request := Request(t, srv, http.MethodGet, basePath+"/ResourceTypes/User", WithBearerToken(validToken))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		resourceType := ReadBodyAs[core.ResourceType](t, response)
		assert.Equal(t, []core.SchemaExtension{{Schema: core.SchemaEnterpriseUser}}, resourceType.SchemaExtensions)
	})

	t.Run("publishes the extension schema", func(t *testing.T) {
		request := Request(t, srv, http.MethodGet, basePath+"/Schemas/"+string(core.SchemaEnterpriseUser), WithBearerToken(validToken))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		schema := ReadBodyAs[core.Schema](t, response)
		assert.NotNil(t, schema.Attributes.Lookup("employeeNumber"))
		assert.Equal(t, basePath+"/Schemas/"+string(core.SchemaEnterpriseUser), schema.Meta.Location)
	})
}

// RFC 7643 7 Schema Definition (writeOnly mutability)
func TestRFC7643WriteOnlyAttributes(t *testing.T) {
	srv := newTestServer(t)

	request := Request(t, srv, http.MethodPost, basePath+"/Users",
		WithBearerToken(validToken),
		WithContentType(protocol.MediaType),
		WithRequestBodyAs(t, core.User{UserName: "bjensen", Password: "t1meMa$heen"}),
	)
	response := Response(t, srv, request)

	require.Equal(t, http.StatusCreated, response.StatusCode)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	assert.NotContains(t, string(body), "password")
}

func TestConcurrentRequests(t *testing.T) {
	srv := newTestServer(t)
	id, _ := create(t, srv, &core.User{UserName: "bjensen"})

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Go(func() {
			create(t, srv, &core.User{UserName: "user" + strconv.Itoa(i)})
		})
		wg.Go(func() {
			request := Request(t, srv, http.MethodGet, basePath+"/Users/"+id, WithBearerToken(validToken))
			assert.Equal(t, http.StatusOK, Response(t, srv, request).StatusCode)
		})
		wg.Go(func() {
			request := Request(t, srv, http.MethodGet, basePath+"/Users", WithBearerToken(validToken))
			assert.Equal(t, http.StatusOK, Response(t, srv, request).StatusCode)
		})
	}
	wg.Wait()

	request := Request(t, srv, http.MethodGet, basePath+"/Users", WithBearerToken(validToken))
	list := ReadBodyAs[protocol.ListResponse[*core.User]](t, Response(t, srv, request))
	assert.Equal(t, 9, list.TotalResults)
}

type recordingRepository struct {
	server.Repository[*core.User]
	replaced []*core.User
}

func (r *recordingRepository) Replace(ctx context.Context, item *core.User) (*core.User, error) {
	r.replaced = append(r.replaced, item)
	return r.Repository.Replace(ctx, item)
}

// RFC 7644 Section 3.5.2: a PATCH changes only the attributes it targets.
func TestRFC7644PatchKeepsWriteOnlyAttributes(t *testing.T) {
	fields := userFields()
	repository := &recordingRepository{Repository: server.NewRepository(basePath+"/Users", core.NewSchema(core.SchemaUser).With(fields.Attributes()...), fields)}
	srv := Server(t, server.New(basePath).WithResource(server.NewResource("User", "/Users", core.SchemaUser, fields).WithRepository(repository)))

	response := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Users",
		WithContentType(protocol.MediaType),
		WithRequestBodyAs(t, core.User{UserName: "bjensen", Password: "t1meMa$heen"}),
	))
	require.Equal(t, http.StatusCreated, response.StatusCode)
	id := ReadBodyAs[core.User](t, response).ID

	response = Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
		WithContentType(protocol.MediaType),
		WithRequestBodyAs(t, protocol.PatchRequest{
			Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
			Operations: []patch.Operation{{Op: patch.OpReplace, Path: "userType", Value: json.RawMessage(`"employee"`)}},
		}),
	))
	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Len(t, repository.replaced, 1)
	assert.Equal(t, "t1meMa$heen", repository.replaced[0].Password)
}
