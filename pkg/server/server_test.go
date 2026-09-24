package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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

const (
	basePath   = "/scim/v2"
	validToken = "s3cr3t"
)

// RFC 6750 2.1 Authorization Request Header Field
func TestRFC6750AuthorizationRequestHeaderField(t *testing.T) {
	t.Run("accepts a valid bearer token", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodGet, basePath+"/Users",
			WithContentType(protocol.MediaType),
			WithHeader("Authorization", "Bearer "+validToken),
		)
		response := Response(t, srv, request)

		assert.Equal(t, http.StatusOK, response.StatusCode)
	})

	t.Run("matches the Bearer scheme case-insensitively", func(t *testing.T) {
		srv := newTestServer(t)

		response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users", WithHeader("Authorization", "bearer "+validToken)))

		assert.Equal(t, http.StatusOK, response.StatusCode)
	})
}

// RFC 6750 3 The WWW-Authenticate Response Header Field
func TestRFC6750TheWWWAuthenticateResponseHeaderField(t *testing.T) {
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
}

// RFC 6750 3.1 Error Codes
func TestRFC6750ErrorCodes(t *testing.T) {
	t.Run("rejects an invalid token with invalid_token", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodGet, basePath+"/Users",
			WithContentType(protocol.MediaType),
			WithHeader("Authorization", "Bearer wrong"),
		)
		response := Response(t, srv, request)

		assert.Equal(t, http.StatusUnauthorized, response.StatusCode)
		assert.Contains(t, response.Header.Get("WWW-Authenticate"), `error="invalid_token"`)
	})

	t.Run("hides why the token is invalid behind a fixed description", func(t *testing.T) {
		srv := bearerServer(t, func(ctx context.Context, _ string) (context.Context, error) {
			return ctx, fmt.Errorf("%w: expired at noon", server.ErrInvalidToken)
		})

		response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users", WithBearerToken("expired")))

		assert.Equal(t, http.StatusUnauthorized, response.StatusCode)
		assert.Equal(t, `Bearer error="invalid_token", error_description="The access token is invalid"`, response.Header.Get("WWW-Authenticate"))
		assert.NotContains(t, ReadBodyAs[scimerrors.Error](t, response).Detail, "noon")
	})

	t.Run("answers a validator failure with 500, no challenge and no internal detail", func(t *testing.T) {
		srv := bearerServer(t, func(ctx context.Context, _ string) (context.Context, error) {
			return ctx, errors.New("dial tcp 10.0.0.1:5432: connection refused")
		})

		response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users", WithBearerToken("anything")))

		assert.Equal(t, http.StatusInternalServerError, response.StatusCode)
		assert.Empty(t, response.Header.Get("WWW-Authenticate"))
		assert.NotContains(t, ReadBodyAs[scimerrors.Error](t, response).Detail, "10.0.0.1")
	})
}

// RFC 7643 2.2 Attribute Characteristics
func TestRFC7643AttributeCharacteristics(t *testing.T) {
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

	t.Run("reports the first missing required attribute in schema order", func(t *testing.T) {
		srv := newTestServer(t)

		for range 20 {
			request := Request(t, srv, http.MethodPost, basePath+"/Widgets",
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBodyAs(t, &widget{Parts: []part{{}}}),
			)
			response := Response(t, srv, request)

			require.Equal(t, http.StatusBadRequest, response.StatusCode)
			require.Equal(t, `"name" is required`, ReadBodyAs[scimerrors.Error](t, response).Detail)
		}
	})

	t.Run("rejects a resource missing a required multi-valued attribute", func(t *testing.T) {
		srv := newTestServer(t, server.WithResource(server.NewResource[*widget]("Kit", "/Kits", kitSchema, kitAttributes()...)))

		request := Request(t, srv, http.MethodPost, basePath+"/Kits",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, &widget{Name: "gear"}),
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

	t.Run("skips a candidate whose optional unique attribute is unset", func(t *testing.T) {
		srv := newTestServer(t)
		createWidget(t, srv, &widget{Name: "a"})
		createWidget(t, srv, &widget{Name: "b"})
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

// RFC 7643 2.5 Unassigned and Null Values
func TestRFC7643UnassignedAndNullValues(t *testing.T) {
	t.Run("treats false as a value and an empty complex or multi-valued attribute as missing", func(t *testing.T) {
		resource := server.NewResource[*core.User]("User", "/Users", core.SchemaUser,
			core.NewAttribute("userName", core.TypeString).AsRequired(),
			core.NewAttribute("active", core.TypeBoolean).AsRequired(),
			core.NewAttribute("name", core.TypeComplex).AsRequired().With(core.NewAttribute("givenName", core.TypeString)),
			core.NewAttribute("emails", core.TypeComplex).AsMultiValued().AsRequired().With(core.NewAttribute("value", core.TypeString)),
		)
		srv := Server(t, server.New(fullServiceProviderConfig(), server.WithResource(resource)))

		for _, test := range []struct {
			name   string
			body   map[string]any
			status int
		}{
			{"accepts false for a required boolean", map[string]any{"userName": "b", "active": false, "name": map[string]any{"givenName": "B"}, "emails": []any{map[string]any{"value": "a@b.com"}}}, http.StatusCreated},
			{"rejects an empty complex value", map[string]any{"userName": "b", "active": true, "name": map[string]any{}, "emails": []any{map[string]any{"value": "a@b.com"}}}, http.StatusBadRequest},
			{"rejects an empty multi-valued value", map[string]any{"userName": "b", "active": true, "name": map[string]any{"givenName": "B"}, "emails": []any{}}, http.StatusBadRequest},
		} {
			t.Run(test.name, func(t *testing.T) {
				request := Request(t, srv, http.MethodPost, basePath+"/Users", WithContentType(protocol.MediaType), WithRequestBodyAs(t, test.body))

				assert.Equal(t, test.status, Response(t, srv, request).StatusCode)
			})
		}
	})
}

// RFC 7643 4.3 Enterprise User Schema Extension
func TestRFC7643EnterpriseUserSchemaExtension(t *testing.T) {
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

	// RFC 7643 Section 6: "schemas" lists every schema, including extensions, used by the resource.

	t.Run("lists the extension in the resource's own schemas array", func(t *testing.T) {
		assert.Equal(t, []core.SchemaURI{core.SchemaUser, core.SchemaEnterpriseUser}, user.Schemas)
	})

	// RFC 7644 Section 3.4.2.2: a URI-qualified attribute name reaches into a schema extension.
	t.Run("filters by a URI-qualified extension attribute", func(t *testing.T) {
		filterQuery := string(core.SchemaEnterpriseUser) + `:employeeNumber eq "1234"`
		path := basePath + "/Users?" + url.Values{"filter": {filterQuery}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Equal(t, 1, list.TotalResults)
		assert.Equal(t, id, list.Resources[0].ID)
	})

	// RFC 7644 Section 3.4.2.3: sortBy also reaches into a schema extension.
	t.Run("sorts by a URI-qualified extension attribute", func(t *testing.T) {
		create(t, srv, &core.User{UserName: "amorris", EnterpriseUser: &core.EnterpriseUser{EmployeeNumber: "0001"}})

		sortBy := string(core.SchemaEnterpriseUser) + ":employeeNumber"
		path := basePath + "/Users?" + url.Values{"sortBy": {sortBy}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.GreaterOrEqual(t, len(list.Resources), 2)
		assert.Equal(t, "amorris", list.Resources[0].UserName)
	})

	// RFC 7644 Section 3.10: every facet of an attribute path, including the URN, is case insensitive.
	t.Run("filters by an extension attribute with an upper-case URN", func(t *testing.T) {
		filterQuery := strings.ToUpper(string(core.SchemaEnterpriseUser)) + `:employeeNumber eq "1234"`
		path := basePath + "/Users?" + url.Values{"filter": {filterQuery}}.Encode()
		response := Response(t, srv, Request(t, srv, http.MethodGet, path, WithBearerToken(validToken)))

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Equal(t, 1, list.TotalResults)
		assert.Equal(t, id, list.Resources[0].ID)
	})

	t.Run("sorts by an extension attribute with an upper-case URN", func(t *testing.T) {
		sortBy := strings.ToUpper(string(core.SchemaEnterpriseUser)) + ":employeeNumber"
		path := basePath + "/Users?" + url.Values{"sortBy": {sortBy}}.Encode()
		response := Response(t, srv, Request(t, srv, http.MethodGet, path, WithBearerToken(validToken)))

		require.Equal(t, http.StatusOK, response.StatusCode)
	})

	// RFC 7644 Section 3.5.2: a PATCH path may be qualified with the schema extension URN.
	t.Run("patches an extension attribute by its URN-qualified path", func(t *testing.T) {
		patchID, _ := create(t, srv, &core.User{UserName: "patched", EnterpriseUser: &core.EnterpriseUser{Department: "eng"}})

		patched := patchUser(t, srv, patchID, patch.Operation{
			Op:    patch.OpReplace,
			Path:  string(core.SchemaEnterpriseUser) + ":department",
			Value: json.RawMessage(`"sales"`),
		})

		require.NotNil(t, patched.EnterpriseUser)
		assert.Equal(t, "sales", patched.EnterpriseUser.Department)
	})

	t.Run("patches an extension object when the path is omitted", func(t *testing.T) {
		patchID, _ := create(t, srv, &core.User{UserName: "merged", EnterpriseUser: &core.EnterpriseUser{EmployeeNumber: "9"}})

		patched := patchUser(t, srv, patchID, patch.Operation{
			Op:    patch.OpReplace,
			Value: json.RawMessage(`{"` + string(core.SchemaEnterpriseUser) + `":{"department":"ops"}}`),
		})

		require.NotNil(t, patched.EnterpriseUser)
		assert.Equal(t, "ops", patched.EnterpriseUser.Department)
		assert.Equal(t, "9", patched.EnterpriseUser.EmployeeNumber)
	})

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

// RFC 7643 5 Service Provider Configuration Schema
func TestRFC7643ServiceProviderConfigurationSchema(t *testing.T) {
	t.Run("PATCH is declined when Patch.Supported is false", func(t *testing.T) {
		config := core.NewServiceProviderConfig(basePath).Sorting().Filtering(protocol.DefaultLimits.MaxCount).Versioning()
		srv := Server(t, server.New(config, standardOptions(t)...))
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		request := Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{{Op: patch.OpReplace, Path: "active", Value: json.RawMessage("true")}},
			}),
		)
		response := Response(t, srv, request)

		assert.Equal(t, http.StatusNotImplemented, response.StatusCode)
	})

	t.Run("filter is declined when Filter.Supported is false", func(t *testing.T) {
		srv := Server(t, server.New(core.NewServiceProviderConfig(basePath), standardOptions(t)...))

		path := basePath + "/Users?" + url.Values{"filter": {`userName eq "bjensen"`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken))
		response := Response(t, srv, request)

		assert.Equal(t, http.StatusNotImplemented, response.StatusCode)
	})

	t.Run("plain listing still works when Filter.Supported is false", func(t *testing.T) {
		srv := Server(t, server.New(core.NewServiceProviderConfig(basePath), standardOptions(t)...))

		request := Request(t, srv, http.MethodGet, basePath+"/Users", WithBearerToken(validToken))
		response := Response(t, srv, request)

		assert.Equal(t, http.StatusOK, response.StatusCode)
	})

	t.Run("sortBy is declined when Sort.Supported is false", func(t *testing.T) {
		srv := Server(t, server.New(core.NewServiceProviderConfig(basePath), standardOptions(t)...))

		path := basePath + "/Users?" + url.Values{"sortBy": {"userName"}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken))
		response := Response(t, srv, request)

		assert.Equal(t, http.StatusNotImplemented, response.StatusCode)
	})

	t.Run("no ETag header when ETag.Supported is false, and a stale If-Match is ignored", func(t *testing.T) {
		config := core.NewServiceProviderConfig(basePath).Patching()
		srv := Server(t, server.New(config, standardOptions(t)...))

		created := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Users",
			WithBearerToken(validToken), WithContentType(protocol.MediaType), WithRequestBodyAs(t, core.User{UserName: "bjensen"})))
		require.Equal(t, http.StatusCreated, created.StatusCode)
		assert.Empty(t, created.Header.Get("ETag"))
		id := ReadBodyAs[core.User](t, created).ID

		replace := Response(t, srv, Request(t, srv, http.MethodPut, basePath+"/Users/"+id,
			WithBearerToken(validToken), WithContentType(protocol.MediaType),
			WithHeader("If-Match", `W/"stale"`), WithRequestBody([]byte(`{"userName":"bjensen2"}`))))
		assert.Equal(t, http.StatusOK, replace.StatusCode)

		patch := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
			WithBearerToken(validToken), WithContentType(protocol.MediaType), WithHeader("If-Match", `W/"stale"`),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{{Op: patch.OpReplace, Path: "userName", Value: json.RawMessage(`"bjensen3"`)}},
			})))
		assert.Equal(t, http.StatusOK, patch.StatusCode)

		del := Response(t, srv, Request(t, srv, http.MethodDelete, basePath+"/Users/"+id,
			WithBearerToken(validToken), WithHeader("If-Match", `W/"stale"`)))
		assert.Equal(t, http.StatusNoContent, del.StatusCode)
	})

	t.Run("runs a minimal server with only CRUD end to end", func(t *testing.T) {
		srv := Server(t, server.New(core.NewServiceProviderConfig(basePath), standardOptions(t)...))
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		get := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users/"+id, WithBearerToken(validToken)))
		assert.Equal(t, http.StatusOK, get.StatusCode)

		del := Response(t, srv, Request(t, srv, http.MethodDelete, basePath+"/Users/"+id, WithBearerToken(validToken)))
		assert.Equal(t, http.StatusNoContent, del.StatusCode)
	})
}

// RFC 7643 7 Schema Definition
func TestRFC7643SchemaDefinition(t *testing.T) {
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

// RFC 7644 2 Authentication and Authorization
func TestRFC7644AuthenticationAndAuthorization(t *testing.T) {
	srv := newTestServer(t)

	request := Request(t, srv, http.MethodPost, basePath+"/Users", WithContentType(protocol.MediaType))
	response := Response(t, srv, request)

	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
}

// RFC 7644 3.3 Creating Resources
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
		assert.Equal(t, []core.SchemaURI{core.SchemaUser, core.SchemaEnterpriseUser}, user.Schemas)
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

	t.Run("rejects a body that does not match the resource", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodPost, basePath+"/Users", WithBearerToken(validToken), WithRequestBody([]byte(`{"userName":5}`)))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidSyntax, ReadBodyAs[scimerrors.Error](t, response).ScimType)
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

	t.Run("rejects a duplicate unique value that needs escaping", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: `b"jensen`})

		request := Request(t, srv, http.MethodPost, basePath+"/Users",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, core.User{UserName: `b"jensen`}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusConflict, response.StatusCode)
		assert.Equal(t, scimerrors.Uniqueness, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7644 Section 3.3: values provided for readOnly attributes SHALL be ignored.
	t.Run("ignores readOnly id and meta", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodPost, basePath+"/Users",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBody([]byte(`{"id":"client-chosen","userName":"alice","meta":{"created":"2000-01-01T00:00:00Z"}}`)),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusCreated, response.StatusCode)
		created := ReadBodyAs[core.User](t, response)
		assert.NotEqual(t, "client-chosen", created.ID)
		assert.NotEqual(t, 2000, created.Meta.Created.Year())
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

	t.Run("admits one of many concurrent creates of a unique value", func(t *testing.T) {
		srv := newTestServer(t)

		const attempts = 16
		statuses := make([]int, attempts)
		var wg sync.WaitGroup
		for i := range attempts {
			wg.Go(func() {
				request := Request(t, srv, http.MethodPost, basePath+"/Users",
					WithBearerToken(validToken),
					WithContentType(protocol.MediaType),
					WithRequestBodyAs(t, core.User{UserName: "bjensen"}),
				)
				statuses[i] = Response(t, srv, request).StatusCode
			})
		}
		wg.Wait()

		created, conflicted := 0, 0
		for _, status := range statuses {
			switch status {
			case http.StatusCreated:
				created++
			case http.StatusConflict:
				conflicted++
			}
		}
		assert.Equal(t, 1, created)
		assert.Equal(t, attempts-1, conflicted)
	})
}

// RFC 7644 3.4.1 Retrieving a Known Resource
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

// RFC 7644 3.4.2 Query Resources
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
		assert.Equal(t, []core.SchemaURI{core.SchemaUser, core.SchemaEnterpriseUser}, list.Resources[0].Schemas)
		assert.NotEmpty(t, list.Resources[0].Meta.Location)
		assert.Equal(t, "bob", list.Resources[1].UserName)
		assert.Equal(t, "carol", list.Resources[2].UserName)
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
}

// RFC 7644 3.4.2.2 Filtering
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

	t.Run("pr matches a complex attribute with a non-empty node", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "bjensen", Name: core.Name{GivenName: "Barbara"}})
		create(t, srv, &core.User{UserName: "jsmith"})

		path := basePath + "/Users?" + url.Values{"filter": {`name pr`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[core.User]](t, response)
		require.Equal(t, 1, list.TotalResults)
		assert.Equal(t, "bjensen", list.Resources[0].UserName)
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

	// RFC 7643 2.4 Multi-Valued Attributes
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

	t.Run("compares a value by the type of its attribute", func(t *testing.T) {
		srv := newTestServer(t)
		createWidget(t, srv, &widget{Name: "bolt", Score: 7, When: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)})
		active := true
		create(t, srv, &core.User{UserName: "bjensen", Active: &active, EnterpriseUser: &core.EnterpriseUser{Department: "Tour"}})

		matches := func(endpoint, filter string) int {
			path := basePath + endpoint + "?" + url.Values{"filter": {filter}}.Encode()
			response := Response(t, srv, Request(t, srv, http.MethodGet, path, WithBearerToken(validToken)))
			require.Equal(t, http.StatusOK, response.StatusCode, filter)
			return ReadBodyAs[protocol.ListResponse[map[string]any]](t, response).TotalResults
		}

		assert.Equal(t, 1, matches("/Widgets", `score gt 5`))
		assert.Equal(t, 1, matches("/Widgets", `when gt "2026-01-01T00:00:00Z"`))
		assert.Equal(t, 1, matches("/Widgets", `meta.created pr`))
		assert.Equal(t, 1, matches("/Widgets", `meta.resourceType eq "Widget"`))
		assert.Equal(t, 1, matches("/Users", `active eq true`))
		assert.Equal(t, 1, matches("/Users", string(core.SchemaEnterpriseUser)+`:department eq "tour"`))
	})
}

// RFC 7644 3.4.2.3 Sorting
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

// RFC 7644 3.4.2.4 Pagination
func TestRFC7644Pagination(t *testing.T) {
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

	t.Run("pages with a custom default count", func(t *testing.T) {
		srv := newTestServer(t, server.DefaultCount(1))
		create(t, srv, &core.User{UserName: "alice"})
		create(t, srv, &core.User{UserName: "bob"})

		request := Request(t, srv, http.MethodGet, basePath+"/Users", WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, 1, ReadBodyAs[protocol.ListResponse[*core.User]](t, response).ItemsPerPage)
	})

	t.Run("caps the count at a custom maximum", func(t *testing.T) {
		config := core.NewServiceProviderConfig(basePath).Sorting().Filtering(2).Patching().Versioning()
		srv := Server(t, server.New(config, standardOptions(t)...))
		create(t, srv, &core.User{UserName: "alice"})
		create(t, srv, &core.User{UserName: "bob"})
		create(t, srv, &core.User{UserName: "carol"})

		path := basePath + "/Users?" + url.Values{"count": {"50"}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, 2, ReadBodyAs[protocol.ListResponse[*core.User]](t, response).ItemsPerPage)
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

// RFC 7644 3.4.2.5 Attributes
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

// RFC 7644 3.4.3 Querying Resources Using HTTP POST
func TestRFC7644QueryingResourcesUsingHTTPPOST(t *testing.T) {
	t.Skip("POST /.search is not registered by server.New; falls through to the generic unknown-path 404")
}

// RFC 7644 3.5.1 Replacing with PUT
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

	// RFC 7644 Section 3.5.1: values provided for readOnly attributes SHALL be ignored.
	t.Run("keeps the stored readOnly meta.created value", func(t *testing.T) {
		srv := newTestServer(t)
		id, etag := create(t, srv, &core.User{UserName: "bjensen"})
		original := ReadBodyAs[core.User](t, Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users/"+id, WithBearerToken(validToken))))

		request := Request(t, srv, http.MethodPut, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithHeader("If-Match", etag),
			WithRequestBody([]byte(`{"userName":"bjensen2","meta":{"created":"2000-01-01T00:00:00Z"}}`)),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		replaced := ReadBodyAs[core.User](t, response)
		assert.Equal(t, "bjensen2", replaced.UserName)
		assert.Equal(t, original.Meta.Created, replaced.Meta.Created)
	})

	t.Run("keeps readOnly values the body leaves out", func(t *testing.T) {
		srv := newTestServer(t)
		id, etag := create(t, srv, &core.User{UserName: "bjensen"})
		original := ReadBodyAs[core.User](t, Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users/"+id, WithBearerToken(validToken))))

		request := Request(t, srv, http.MethodPut, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithHeader("If-Match", etag),
			WithRequestBody([]byte(`{"userName":"bjensen3"}`)),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, original.Meta.Created, ReadBodyAs[core.User](t, response).Meta.Created)
	})
}

// RFC 7644 3.5.2 Modifying with PATCH
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

	// RFC 7644 Section 3.5.2: a client MUST NOT modify a readOnly attribute.
	t.Run("rejects a patch that targets a readOnly attribute", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		for _, body := range []string{
			`{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[{"op":"replace","path":"id","value":"x"}]}`,
			`{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[{"op":"replace","value":{"meta":{"version":"x"}}}]}`,
		} {
			request := Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBody([]byte(body)),
			)
			response := Response(t, srv, request)

			require.Equal(t, http.StatusBadRequest, response.StatusCode)
			assert.Equal(t, scimerrors.Mutability, ReadBodyAs[scimerrors.Error](t, response).ScimType)
		}
	})

	// RFC 7644 Section 3.5.2: the body carries the PatchOp schema and one or more operations.
	t.Run("rejects a patch without the PatchOp schema or operations", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		for _, body := range []string{
			`{"Operations":[{"op":"replace","path":"userName","value":"x"}]}`,
			`{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[]}`,
		} {
			request := Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBody([]byte(body)),
			)
			response := Response(t, srv, request)

			require.Equal(t, http.StatusBadRequest, response.StatusCode)
			assert.Equal(t, scimerrors.InvalidSyntax, ReadBodyAs[scimerrors.Error](t, response).ScimType)
		}
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

	t.Run("rejects changing an immutable value but allows adding and removing members", func(t *testing.T) {
		emails := core.NewAttribute("emails", core.TypeComplex).AsMultiValued().With(
			core.NewAttribute("type", core.TypeString).AsImmutable(),
			core.NewAttribute("value", core.TypeString),
		)
		resource := server.NewResource[*core.User]("User", "/Users", core.SchemaUser, core.NewAttribute("userName", core.TypeString), emails).
			WithExtension(core.SchemaEnterpriseUser, core.NewAttribute("employeeNumber", core.TypeString).AsImmutable())
		srv := Server(t, server.New(fullServiceProviderConfig(), server.WithResource(resource)))
		extension := string(core.SchemaEnterpriseUser)

		for _, test := range []struct {
			name   string
			op     patch.Operation
			status int
		}{
			{"rejects a changed extension value", patch.Operation{Op: patch.OpReplace, Path: extension + ":employeeNumber", Value: json.RawMessage(`"E2"`)}, http.StatusBadRequest},
			{"accepts the same extension value", patch.Operation{Op: patch.OpReplace, Path: extension + ":employeeNumber", Value: json.RawMessage(`"E1"`)}, http.StatusOK},
			{"accepts adding a member", patch.Operation{Op: patch.OpAdd, Path: "emails", Value: json.RawMessage(`[{"type":"home","value":"c@d.com"}]`)}, http.StatusOK},
			{"accepts removing a member", patch.Operation{Op: patch.OpRemove, Path: `emails[value eq "a@b.com"]`}, http.StatusOK},
		} {
			t.Run(test.name, func(t *testing.T) {
				user := &core.User{UserName: "bjensen", Emails: []core.Email{{Type: "work", Value: "a@b.com"}}, EnterpriseUser: &core.EnterpriseUser{EmployeeNumber: "E1"}}
				created := ReadBodyAs[core.User](t, Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Users", WithContentType(protocol.MediaType), WithRequestBodyAs(t, user))))

				request := Request(t, srv, http.MethodPatch, basePath+"/Users/"+created.ID,
					WithContentType(protocol.MediaType),
					WithRequestBodyAs(t, protocol.PatchRequest{Schemas: []core.SchemaURI{protocol.SchemaPatchOp}, Operations: []patch.Operation{test.op}}),
				)

				assert.Equal(t, test.status, Response(t, srv, request).StatusCode)
			})
		}
	})
}

// RFC 7644 3.5.2.1 Add Operation
func TestRFC7644AddOperation(t *testing.T) {
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
}

// RFC 7644 3.5.2.2 Remove Operation
func TestRFC7644RemoveOperation(t *testing.T) {
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

// RFC 7644 3.5.2.3 Replace Operation
func TestRFC7644ReplaceOperation(t *testing.T) {
	// RFC 7644 Section 3.5.2.3: sub-attributes that are not specified in the "value" parameter are left unchanged.
	t.Run("merges the sub-attributes of a replaced complex attribute", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen", Name: core.Name{GivenName: "Barbara", FamilyName: "Jensen"}})

		patched := patchUser(t, srv, id, patch.Operation{Op: patch.OpReplace, Path: "name", Value: json.RawMessage(`{"givenName":"Babs"}`)})

		assert.Equal(t, core.Name{GivenName: "Babs", FamilyName: "Jensen"}, patched.Name)
	})
}

// RFC 7644 3.6 Deleting Resources
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

// RFC 7644 3.7 Bulk Operations
func TestRFC7644BulkOperations(t *testing.T) {
	t.Skip("Bulk is not registered by server.New; falls through to the generic unknown-path 404")
}

// RFC 7644 3.11 "/Me" Authenticated Subject Alias
func TestRFC7644MeAuthenticatedSubjectAlias(t *testing.T) {
	t.Skip("/Me is not implemented by server.New; requests to it 404 through the generic unknown-path behavior")
}

// RFC 7644 3.12 HTTP Status and Error Response Handling
func TestRFC7644HTTPStatusAndErrorResponseHandling(t *testing.T) {
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

// RFC 7644 3.14 Versioning Resources
func TestRFC7644VersioningResources(t *testing.T) {
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

	t.Run("If-Match * matches any version", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		request := Request(t, srv, http.MethodDelete, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithHeader("If-Match", "*"),
		)
		response := Response(t, srv, request)

		assert.Equal(t, http.StatusNoContent, response.StatusCode)
	})

	t.Run("serves concurrent reads and writes without losing a resource", func(t *testing.T) {
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
			wg.Go(func() {
				request := Request(t, srv, http.MethodPut, basePath+"/Users/"+id,
					WithBearerToken(validToken),
					WithContentType(protocol.MediaType),
					WithRequestBody([]byte(`{"userName":"bjensen"}`)),
				)
				assert.Equal(t, http.StatusOK, Response(t, srv, request).StatusCode)
			})
			wg.Go(func() {
				request := Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
					WithBearerToken(validToken),
					WithContentType(protocol.MediaType),
					WithRequestBodyAs(t, protocol.PatchRequest{
						Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
						Operations: []patch.Operation{{Op: patch.OpReplace, Path: "active", Value: json.RawMessage("true")}},
					}),
				)
				assert.Contains(t, []int{http.StatusOK, http.StatusPreconditionFailed}, Response(t, srv, request).StatusCode)
			})
		}
		wg.Wait()

		request := Request(t, srv, http.MethodGet, basePath+"/Users", WithBearerToken(validToken))
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, Response(t, srv, request))
		assert.Equal(t, 9, list.TotalResults)
	})
}

// RFC 7644 4 Service Provider Configuration Endpoints (/ServiceProviderConfig)
func TestRFC7644ServiceProviderConfiguration(t *testing.T) {
	t.Run("advertises capabilities and authentication schemes", func(t *testing.T) {
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
	})

	t.Run("advertises its own location", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodGet, basePath+"/ServiceProviderConfig", WithBearerToken(validToken))
		config := ReadBodyAs[core.ServiceProviderConfig](t, Response(t, srv, request))

		assert.Equal(t, basePath+"/ServiceProviderConfig", config.Meta.Location)
	})

	t.Run("advertises the custom max results configured via Filtering", func(t *testing.T) {
		config := core.NewServiceProviderConfig(basePath).Sorting().Filtering(2).Patching().Versioning()
		srv := Server(t, server.New(config, standardOptions(t)...))

		request := Request(t, srv, http.MethodGet, basePath+"/ServiceProviderConfig", WithBearerToken(validToken))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, 2, ReadBodyAs[core.ServiceProviderConfig](t, response).Filter.MaxResults)
	})

	// RFC 7643 Section 5: the config a caller builds is exactly what the server advertises.
	t.Run("advertises a mutated config directly", func(t *testing.T) {
		config := core.NewServiceProviderConfig(basePath).Patching().Filtering(protocol.DefaultLimits.MaxCount)
		config.DocumentationURI = "https://example.com/help/scim.html"
		srv := Server(t, server.New(config, standardOptions(t)...))

		request := Request(t, srv, http.MethodGet, basePath+"/ServiceProviderConfig", WithBearerToken(validToken))
		advertised := ReadBodyAs[core.ServiceProviderConfig](t, Response(t, srv, request))

		assert.Equal(t, "https://example.com/help/scim.html", advertised.DocumentationURI)
		assert.False(t, advertised.Sort.Supported)
		assert.True(t, advertised.Patch.Supported)
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

// RFC 7644 4 Service Provider Configuration Endpoints (/Schemas)
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

	// RFC 7644 Section 4: a "filter" on this endpoint SHOULD be rejected with 403, not silently ignored.
	t.Run("rejects a filter query parameter with 403", func(t *testing.T) {
		srv := newTestServer(t)

		path := basePath + "/Schemas?" + url.Values{"filter": {`id eq "x"`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken))
		response := Response(t, srv, request)

		assert.Equal(t, http.StatusForbidden, response.StatusCode)
	})
}

// RFC 7644 4 Service Provider Configuration Endpoints (/ResourceTypes)
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

	// RFC 7644 Section 4: a "filter" on this endpoint SHOULD be rejected with 403, not silently ignored.
	t.Run("rejects a filter query parameter with 403", func(t *testing.T) {
		srv := newTestServer(t)

		path := basePath + "/ResourceTypes?" + url.Values{"filter": {`id eq "x"`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken))
		response := Response(t, srv, request)

		assert.Equal(t, http.StatusForbidden, response.StatusCode)
	})
}

func patchUser(t *testing.T, srv *httptest.Server, id string, operations ...patch.Operation) core.User {
	t.Helper()

	request := Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
		WithBearerToken(validToken),
		WithContentType(protocol.MediaType),
		WithRequestBodyAs(t, protocol.PatchRequest{
			Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
			Operations: operations,
		}),
	)
	response := Response(t, srv, request)
	require.Equal(t, http.StatusOK, response.StatusCode)
	return ReadBodyAs[core.User](t, response)
}
