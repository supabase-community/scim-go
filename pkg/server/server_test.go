package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
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

// RFC 6750 2.1 Authorization Request Header Field
func TestRFC6750AuthorizationRequestHeaderField(t *testing.T) {
	// RFC 6750 Section 2.1: resource servers MUST support a bearer token in the "Authorization" header with the "Bearer" scheme.
	t.Run("accepts a valid bearer token", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodGet, basePath+"/Users",
			WithContentType(protocol.MediaType),
			WithHeader("Authorization", "Bearer "+validToken),
		)
		response := Response(t, srv, request)

		assert.Equal(t, http.StatusOK, response.StatusCode)
	})

	// RFC 7235 Section 2.1: a case-insensitive token identifies the authentication scheme.
	t.Run("matches the Bearer scheme case-insensitively", func(t *testing.T) {
		srv := newTestServer(t)

		response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users", WithHeader("Authorization", "bearer "+validToken)))

		assert.Equal(t, http.StatusOK, response.StatusCode)
	})
}

// RFC 6750 2.2 Form-Encoded Body Parameter
func TestRFC6750FormEncodedBodyParameter(t *testing.T) {
	t.Skip("MAY: SCIM request bodies are JSON, so a form-encoded access_token is not supported")
}

// RFC 6750 2.3 URI Query Parameter
func TestRFC6750URIQueryParameter(t *testing.T) {
	t.Skip("MAY: an access_token query parameter is not supported")
}

// RFC 6750 3 The WWW-Authenticate Response Header Field
func TestRFC6750TheWWWAuthenticateResponseHeaderField(t *testing.T) {
	// RFC 6750 Section 3.1: if the request lacks any authentication information, the server SHOULD NOT include an error code.
	t.Run("omits error info when no Authorization header is present", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodGet, basePath+"/Users", WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		assert.Equal(t, http.StatusUnauthorized, response.StatusCode)
		assert.Equal(t, `Bearer realm="scim"`, response.Header.Get("WWW-Authenticate"))
	})

	// RFC 6750 Section 3.1: a request using an unsupported authentication method SHOULD NOT get an error code.
	t.Run("omits error info for a non-Bearer scheme", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodGet, basePath+"/Users",
			WithContentType(protocol.MediaType),
			WithHeader("Authorization", "Basic dXNlcjpwYXNz"),
		)
		response := Response(t, srv, request)

		assert.Equal(t, http.StatusUnauthorized, response.StatusCode)
		assert.Equal(t, `Bearer realm="scim"`, response.Header.Get("WWW-Authenticate"))
	})
}

// RFC 6750 3.1 Error Codes
func TestRFC6750ErrorCodes(t *testing.T) {
	// RFC 6750 Section 3.1: invalid_token when the access token is expired, revoked, malformed, or invalid, with 401.
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

	// RFC 6750 Section 3.1: invalid_request when the request is missing a required parameter or is malformed, with 400.
	t.Run("rejects a Bearer scheme without a token with invalid_request", func(t *testing.T) {
		srv := newTestServer(t)

		response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users", WithHeader("Authorization", "Bearer")))

		assert.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Contains(t, response.Header.Get("WWW-Authenticate"), `error="invalid_request"`)
	})

	// RFC 6750 Section 3: the server MAY include an "error_description" that is not meant to be displayed to end-users.
	t.Run("hides why the token is invalid behind a fixed description", func(t *testing.T) {
		srv := newTestServer(t)

		response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users", WithBearerToken(expiredToken)))

		assert.Equal(t, http.StatusUnauthorized, response.StatusCode)
		assert.Equal(t, `Bearer realm="scim", error="invalid_token", error_description="The access token is invalid"`, response.Header.Get("WWW-Authenticate"))
		assert.NotContains(t, ReadBodyAs[scimerrors.Error](t, response).Detail, "noon")
	})

	// RFC 7644 Section 3.12: 500 for an internal error.
	t.Run("answers a validator failure with 500, no challenge and no internal detail", func(t *testing.T) {
		srv := newTestServer(t)

		response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users", WithBearerToken(unreachableToken)))

		assert.Equal(t, http.StatusInternalServerError, response.StatusCode)
		assert.Empty(t, response.Header.Get("WWW-Authenticate"))
		assert.NotContains(t, ReadBodyAs[scimerrors.Error](t, response).Detail, "10.0.0.1")
	})

	// RFC 6750 Section 3.1: insufficient_scope when the request requires higher privileges than the token provides, with 403.
	t.Run("rejects a token without the required scope with insufficient_scope", func(t *testing.T) {
		t.Skip("SHOULD: there is no scope model, so insufficient_scope (403) is never issued")
	})
}

// RFC 7643 2.1 Attributes
func TestRFC7643Attributes(t *testing.T) {
	// RFC 7643 Section 2.1: attribute names are case insensitive and the character set is US-ASCII.
	t.Run("reads attribute names in a request body case-insensitively", func(t *testing.T) {
		srv := newTestServer(t)

		response := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Users",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBody([]byte(`{"USERNAME":"bjensen"}`)),
		))

		require.Equal(t, http.StatusCreated, response.StatusCode)
		assert.Equal(t, "bjensen", ReadBodyAs[core.User](t, response).UserName)
	})

	// RFC 7643 Section 2.1: attribute names are case insensitive, so a changed immutable sub-attribute keyed with different case is still rejected.
	t.Run("rejects a changed immutable sub-attribute keyed with different case", func(t *testing.T) {
		t.Skip("MUST: member sub-attribute immutability is left to the Group repository, so the default server accepts the change")
	})
}

// RFC 7643 2.2 Attribute Characteristics
func TestRFC7643AttributeCharacteristics(t *testing.T) {
	// RFC 7643 Section 7: "required" specifies whether or not the attribute is required.
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

	// RFC 7643 Section 7: "required" specifies whether or not the attribute is required.
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

	// RFC 7643 Section 7: "required" specifies whether or not the attribute is required.
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

	// RFC 7643 Section 7: "required" specifies whether or not the attribute is required.
	t.Run("rejects a resource missing a required multi-valued attribute", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodPost, basePath+"/Kits",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, map[string]any{"active": true, "name": map[string]any{"givenName": "gear"}}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidValue, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7643 Section 7: "required" specifies whether or not the attribute is required.
	t.Run("requires a sub-attribute only when its optional parent is present", func(t *testing.T) {
		srv := newTestServer(t, withOption(server.WithResource(server.NewResource[*kit]("Crate", "/Crates", "urn:test:crate",
			core.NewAttribute("name", core.TypeComplex).With(core.NewAttribute("givenName", core.TypeString).AsRequired()),
		))))

		for body, status := range map[string]int{
			`{"schemas":["urn:test:crate"]}`:             http.StatusCreated,
			`{"schemas":["urn:test:crate"],"name":{}}`:   http.StatusBadRequest,
			`{"schemas":["urn:test:crate"],"name":null}`: http.StatusCreated,
		} {
			response := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Crates",
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBody([]byte(body)),
			))

			assert.Equal(t, status, response.StatusCode, body)
		}
	})

	// RFC 7643 Section 7: "canonicalValues" is a collection of suggested canonical values that MAY be used.
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

	// RFC 7643 Section 7: "canonicalValues" is a collection of suggested canonical values that MAY be used.
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

	// RFC 7643 Section 2.5: unassigned attributes and the null value SHALL be considered equivalent.
	t.Run("skips a candidate whose optional unique attribute is unset", func(t *testing.T) {
		srv := newTestServer(t)
		createWidget(t, srv, &widget{Name: "a"})
		createWidget(t, srv, &widget{Name: "b"})
	})

	// RFC 7643 Section 7: "caseExact" specifies whether or not a string attribute is case sensitive.
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

// RFC 7643 2.3.3 Decimal
func TestRFC7643Decimal(t *testing.T) {
	// RFC 7643 Section 2.3.3: a decimal is a real number, so 1.5 and 1.50 are the same value.
	t.Run("keeps an immutable value written with a different number of digits", func(t *testing.T) {
		srv := newTestServer(t)
		id := createWidget(t, srv, &widget{Name: "gizmo", Weight: 1.5})["id"].(string)

		request := Request(t, srv, http.MethodPatch, basePath+"/Widgets/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{{Op: patch.OpReplace, Path: "weight", Value: json.RawMessage(`1.50`)}},
			}),
		)

		assert.Equal(t, http.StatusOK, Response(t, srv, request).StatusCode)
	})
}

// RFC 7643 2.3.5 DateTime
func TestRFC7643DateTime(t *testing.T) {
	// RFC 7643 Section 2.3.5: a valid xsd:dateTime MUST include both a date and a time; its time zone offset is optional.
	t.Run("accepts a value without a time zone offset", func(t *testing.T) {
		srv := newTestServer(t)
		createWidget(t, srv, &widget{Name: "early", When: time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC)})
		createWidget(t, srv, &widget{Name: "late", When: time.Date(2021, 6, 1, 0, 0, 0, 0, time.UTC)})
		search := func(filter string) *http.Response {
			path := basePath + "/Widgets?" + url.Values{"filter": {filter}}.Encode()
			return Response(t, srv, Request(t, srv, http.MethodGet, path, WithBearerToken(validToken)))
		}

		response := search(`when gt "2020-12-31T00:00:00"`)
		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[map[string]any]](t, response)
		require.Len(t, list.Resources, 1)
		assert.Equal(t, "late", list.Resources[0]["Name"])

		response = search(`when gt "2020-12-31"`)
		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidValue, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})
}

// RFC 7643 2.3.6 Binary
func TestRFC7643Binary(t *testing.T) {
	// RFC 7643 Section 2.3.6: a binary value MUST be base64 encoded as specified in Section 4 of RFC 4648.
	t.Run("rejects a value that is not base64 on create, replace, and patch", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})
		user := core.User{UserName: "jsmith", X509Certificates: []core.X509Certificate{{Value: "!!!not base64"}}}
		add := protocol.PatchRequest{
			Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
			Operations: []patch.Operation{{Op: patch.OpAdd, Path: "x509Certificates", Value: json.RawMessage(`[{"value":"!!!not base64"}]`)}},
		}

		for _, test := range []struct {
			method, path string
			body         Option[*http.Request]
		}{
			{http.MethodPost, "/Users", WithRequestBodyAs(t, user)},
			{http.MethodPut, "/Users/" + id, WithRequestBodyAs(t, user)},
			{http.MethodPatch, "/Users/" + id, WithRequestBodyAs(t, add)},
		} {
			response := Response(t, srv, Request(t, srv, test.method, basePath+test.path,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				test.body,
			))

			require.Equal(t, http.StatusBadRequest, response.StatusCode, test.method)
			assert.Equal(t, scimerrors.InvalidValue, ReadBodyAs[scimerrors.Error](t, response).ScimType, test.method)
		}
	})

	// RFC 7643 Section 2.3.6: trailing padding characters MAY be omitted ("=").
	t.Run("accepts a value with its trailing padding omitted", func(t *testing.T) {
		srv := newTestServer(t)

		create(t, srv, &core.User{UserName: "bjensen", X509Certificates: []core.X509Certificate{{Value: "YQ"}}})
	})
}

// RFC 7643 2.4 Multi-Valued Attributes
func TestRFC7643MultiValuedAttributes(t *testing.T) {
	// RFC 7643 Section 2.4: the primary attribute value "true" MUST appear no more than once.
	t.Run("rejects more than one primary value on create and replace", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})
		primary := true
		emails := []core.Email{{Value: "a@example.com", Primary: &primary}, {Value: "b@example.com", Primary: &primary}}

		for method, path := range map[string]string{http.MethodPost: "/Users", http.MethodPut: "/Users/" + id} {
			response := Response(t, srv, Request(t, srv, method, basePath+path,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBodyAs(t, core.User{UserName: "jsmith", Emails: emails}),
			))

			assert.Equal(t, http.StatusBadRequest, response.StatusCode, method)
		}
	})
}

// RFC 7643 2.5 Unassigned and Null Values
func TestRFC7643UnassignedAndNullValues(t *testing.T) {
	// RFC 7643 Section 2.5: unassigned attributes, the null value, or an empty array SHALL be considered equivalent in "state".
	t.Run("treats false as a value and an empty complex or multi-valued attribute as missing", func(t *testing.T) {
		srv := newTestServer(t)

		for _, test := range []struct {
			name   string
			body   map[string]any
			status int
		}{
			{"accepts false for a required boolean", map[string]any{"active": false, "name": map[string]any{"givenName": "B"}, "parts": []any{map[string]any{"serial": "s-1"}}}, http.StatusCreated},
			{"rejects an empty complex value", map[string]any{"active": true, "name": map[string]any{}, "parts": []any{map[string]any{"serial": "s-1"}}}, http.StatusBadRequest},
			{"rejects an empty multi-valued value", map[string]any{"active": true, "name": map[string]any{"givenName": "B"}, "parts": []any{}}, http.StatusBadRequest},
		} {
			t.Run(test.name, func(t *testing.T) {
				request := Request(t, srv, http.MethodPost, basePath+"/Kits", WithBearerToken(validToken), WithContentType(protocol.MediaType), WithRequestBodyAs(t, test.body))

				assert.Equal(t, test.status, Response(t, srv, request).StatusCode)
			})
		}
	})

	// RFC 7644 Section 3.5.1: the service provider MAY assume that existing values of omitted readWrite attributes are to be cleared.
	t.Run("clears an attribute a replace leaves out", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen", UserType: "employee"})

		response := Response(t, srv, Request(t, srv, http.MethodPut, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, core.User{UserName: "bjensen"}),
		))

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Empty(t, ReadBodyAs[core.User](t, response).UserType)
	})
}

// RFC 7643 3.1 Common Attributes
func TestRFC7643CommonAttributes(t *testing.T) {
	// RFC 7643 Section 3.1: if a resource has never been modified, lastModified MUST be the same as created.
	t.Run("sets lastModified to created on create and keeps externalId", func(t *testing.T) {
		srv := newTestServer(t)

		response := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Users",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, core.User{ExternalID: "hr-42", UserName: "bjensen"}),
		))

		require.Equal(t, http.StatusCreated, response.StatusCode)
		created := ReadBodyAs[core.User](t, response)
		assert.Equal(t, created.Meta.Created, created.Meta.LastModified)
		assert.Equal(t, "hr-42", created.ExternalID)
	})

	// RFC 7643 Section 3.1: meta.location MUST be the same as the "Content-Location" HTTP response header.
	t.Run("sets the Content-Location header to meta.location", func(t *testing.T) {
		srv := newTestServer(t)
		created := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Users",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, core.User{UserName: "bjensen"}),
		))
		require.Equal(t, http.StatusCreated, created.StatusCode)
		location := ReadBodyAs[core.User](t, created).Meta.Location
		require.NotEmpty(t, location)
		assert.Equal(t, location, created.Header.Get("Content-Location"))

		for _, request := range []*http.Request{
			Request(t, srv, http.MethodGet, location, WithBearerToken(validToken)),
			Request(t, srv, http.MethodGet, location+"?attributes=userName", WithBearerToken(validToken)),
			Request(t, srv, http.MethodPut, location,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBodyAs(t, core.User{UserName: "bjensen", DisplayName: "Babs"}),
			),
			Request(t, srv, http.MethodPatch, location,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBodyAs(t, protocol.PatchRequest{
					Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
					Operations: []patch.Operation{{Op: patch.OpReplace, Path: "displayName", Value: json.RawMessage(`"Barbara"`)}},
				}),
			),
		} {
			response := Response(t, srv, request)
			require.Equal(t, http.StatusOK, response.StatusCode, request.Method)
			assert.Equal(t, location, response.Header.Get("Content-Location"), request.Method)
		}
	})

	// RFC 7643 Section 3.1: resourceType has a mutability of "readOnly" and "caseExact" as "true".
	t.Run("compares meta.resourceType case exactly", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "bjensen"})

		matches := func(filter string) int {
			path := basePath + "/Users?" + url.Values{"filter": {filter}}.Encode()
			response := Response(t, srv, Request(t, srv, http.MethodGet, path, WithBearerToken(validToken)))
			require.Equal(t, http.StatusOK, response.StatusCode, filter)
			return ReadBodyAs[protocol.ListResponse[map[string]any]](t, response).TotalResults
		}

		assert.Equal(t, 1, matches(`meta.resourceType eq "User"`))
		assert.Equal(t, 0, matches(`meta.resourceType eq "user"`))
	})

	// RFC 7643 Section 3.1: meta.location MUST be the same as the "Content-Location" HTTP response header.
	t.Run("sets the Content-Location header on discovery documents", func(t *testing.T) {
		srv := newTestServer(t)

		for _, path := range []string{"/ServiceProviderConfig", "/Schemas/" + string(core.SchemaUser)} {
			response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+path, WithBearerToken(validToken)))
			require.Equal(t, http.StatusOK, response.StatusCode, path)
			assert.Equal(t, basePath+path, response.Header.Get("Content-Location"), path)
		}
	})

	// RFC 7643 Section 3.1: meta.location is the URI of the resource being returned.
	t.Run("builds meta.location from the base URL while routing under the base path", func(t *testing.T) {
		baseURL := "https://example.com/auth/v1" + basePath
		srv := newTestServer(t, withOption(server.WithBaseURL(baseURL)))

		for _, path := range []string{
			"/ServiceProviderConfig",
			"/Schemas/" + string(core.SchemaUser),
			"/Schemas/" + string(core.SchemaEnterpriseUser),
		} {
			response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+path, WithBearerToken(validToken)))
			require.Equal(t, http.StatusOK, response.StatusCode, path)
			assert.Equal(t, baseURL+path, ReadBodyAs[map[string]any](t, response)["meta"].(map[string]any)["location"], path)
			assert.Equal(t, baseURL+path, response.Header.Get("Content-Location"), path)
		}

		created := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Groups",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, core.Group{DisplayName: "Admins"}),
		))
		require.Equal(t, http.StatusCreated, created.StatusCode)
		group := ReadBodyAs[core.Group](t, created)
		assert.Equal(t, baseURL+"/Groups/"+group.ID, group.Meta.Location)
		assert.Equal(t, group.Meta.Location, created.Header.Get("Location"))
	})
}

// RFC 7643 4.2 "Group" Resource Schema
func TestRFC7643GroupResourceSchema(t *testing.T) {
	// RFC 7643 Section 4.2: displayName is a human-readable name for the Group and is REQUIRED.
	t.Run("creates, fetches, and requires displayName for a group", func(t *testing.T) {
		srv := newTestServer(t)
		post := func(group core.Group) *http.Response {
			return Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Groups",
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBodyAs(t, group),
			))
		}

		created := post(core.Group{DisplayName: "Tour Guides"})
		require.Equal(t, http.StatusCreated, created.StatusCode)
		id := ReadBodyAs[core.Group](t, created).ID
		fetched := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Groups/"+id, WithBearerToken(validToken)))
		require.Equal(t, http.StatusOK, fetched.StatusCode)
		assert.Equal(t, "Tour Guides", ReadBodyAs[core.Group](t, fetched).DisplayName)
		assert.Equal(t, http.StatusBadRequest, post(core.Group{}).StatusCode)
	})

	// RFC 7643 Section 4.2: service providers MAY require clients to provide a non-empty member value.
	t.Run("rejects a member without a value", func(t *testing.T) {
		srv := newTestServer(t)
		id := createGroup(t, srv, &core.Group{DisplayName: "Tour Guides"}).ID

		for _, member := range []string{`{"type":"User"}`, `{"value":"","type":"User"}`} {
			group := map[string]any{"displayName": "Tour Guides", "members": []json.RawMessage{json.RawMessage(member)}}
			for _, request := range []*http.Request{
				Request(t, srv, http.MethodPost, basePath+"/Groups", WithBearerToken(validToken), WithContentType(protocol.MediaType), WithRequestBodyAs(t, group)),
				Request(t, srv, http.MethodPut, basePath+"/Groups/"+id, WithBearerToken(validToken), WithContentType(protocol.MediaType), WithRequestBodyAs(t, group)),
				patchRequest(t, srv, "Groups/"+id, patch.Operation{Op: patch.OpAdd, Path: "members", Value: json.RawMessage("[" + member + "]")}),
			} {
				response := Response(t, srv, request)
				require.Equal(t, http.StatusBadRequest, response.StatusCode, request.Method+" "+member)
				assert.Equal(t, scimerrors.InvalidValue, ReadBodyAs[scimerrors.Error](t, response).ScimType)
			}
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

	// RFC 7643 Section 3.3: the "schemas" attribute MAY contain additional values indicating extended schemas that are in use.

	// RFC 7643 Section 3.3: the "schemas" attribute MAY contain additional values indicating extended schemas that are in use.
	t.Run("lists the extension in the resource's own schemas array", func(t *testing.T) {
		assert.Equal(t, []core.SchemaURI{core.SchemaUser, core.SchemaEnterpriseUser}, user.Schemas)
	})

	// RFC 7643 Section 3: "schemas" indicates the schemas that define the attributes present in the current JSON structure.
	t.Run("omits the extension from schemas when no extension data is set", func(t *testing.T) {
		plainID, _ := create(t, srv, &core.User{UserName: "plain"})

		request := Request(t, srv, http.MethodGet, basePath+"/Users/"+plainID, WithBearerToken(validToken))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, []core.SchemaURI{core.SchemaUser}, ReadBodyAs[core.User](t, response).Schemas)
	})

	// RFC 7643 Section 3: "schemas" indicates the schemas that define the attributes present in the current JSON structure.
	t.Run("drops the extension from schemas once a replace removes its data", func(t *testing.T) {
		removeID, etag := create(t, srv, &core.User{UserName: "removable", EnterpriseUser: &core.EnterpriseUser{Department: "ops"}})

		response := Response(t, srv, Request(t, srv, http.MethodPut, basePath+"/Users/"+removeID,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithHeader("If-Match", etag),
			WithRequestBodyAs(t, core.User{UserName: "removable"}),
		))

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, []core.SchemaURI{core.SchemaUser}, ReadBodyAs[core.User](t, response).Schemas)
	})

	// RFC 7643 Section 2.5: an attribute holding null is unassigned, so an extension holding only nulls is not present.
	t.Run("omits the extension from schemas when a PUT sends only null extension values", func(t *testing.T) {
		for i, extension := range []string{`{"department":null}`, `{"manager":{"value":null}}`} {
			userName := "nulled-" + strconv.Itoa(i)
			nullID, _ := create(t, srv, &core.User{UserName: userName})

			response := Response(t, srv, Request(t, srv, http.MethodPut, basePath+"/Users/"+nullID,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBody([]byte(`{"userName":"`+userName+`","urn:ietf:params:scim:schemas:extension:enterprise:2.0:User":`+extension+`}`)),
			))

			require.Equal(t, http.StatusOK, response.StatusCode, extension)
			assert.Equal(t, []core.SchemaURI{core.SchemaUser}, ReadBodyAs[core.User](t, response).Schemas, extension)
		}
	})

	// RFC 7643 Section 3: "schemas" indicates the schemas that define the attributes present in the current JSON structure.
	t.Run("stamps schemas from the data regardless of the schemas a client sends", func(t *testing.T) {
		for i, schemas := range []string{
			``,
			`"schemas":[],`,
			`"schemas":["urn:unknown"],`,
			`"schemas":["http://schemas.microsoft.com/2006/11/ResourceManagement/ADSCIM/2.0/User"],`,
			`"schemas":["urn:ietf:params:scim:schemas:core:2.0:Group"],`,
			`"schemas":["urn:ietf:params:scim:schemas:core:2.0:User","urn:ietf:params:scim:schemas:core:2.0:User"],`,
			`"schemas":["urn:ietf:params:scim:schemas:core:2.0:User","urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"],`,
		} {
			response := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Users",
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBody([]byte(`{`+schemas+`"userName":"stamped-`+strconv.Itoa(i)+`"}`)),
			))

			require.Equal(t, http.StatusCreated, response.StatusCode, schemas)
			assert.Equal(t, []core.SchemaURI{core.SchemaUser}, ReadBodyAs[core.User](t, response).Schemas, schemas)
		}
	})

	// RFC 7643 Section 6: a resource type lists the schema extensions it accepts.
	t.Run("advertises the extension on the resource type", func(t *testing.T) {
		request := Request(t, srv, http.MethodGet, basePath+"/ResourceTypes/User", WithBearerToken(validToken))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		resourceType := ReadBodyAs[core.ResourceType](t, response)
		assert.Equal(t, []core.SchemaExtension{{Schema: core.SchemaEnterpriseUser}}, resourceType.SchemaExtensions)
	})

	// RFC 7643 Section 6: if "required" is false, a resource of this type MAY omit this schema extension.
	t.Run("requires an extension attribute only when the optional extension is present", func(t *testing.T) {
		srv := newTestServer(t, withOption(server.WithResource(server.NewResource[*kit]("Box", "/Boxes", "urn:test:box").
			WithExtension("urn:test:label", core.NewAttribute("number", core.TypeString).AsRequired(), core.NewAttribute("color", core.TypeString)),
		)))

		for body, status := range map[string]int{
			`{"schemas":["urn:test:box"]}`:                                                   http.StatusCreated,
			`{"schemas":["urn:test:box","urn:test:label"],"urn:test:label":{}}`:              http.StatusCreated,
			`{"schemas":["urn:test:box","urn:test:label"],"urn:test:label":{"color":"red"}}`: http.StatusBadRequest,
		} {
			response := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Boxes",
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBody([]byte(body)),
			))

			assert.Equal(t, status, response.StatusCode, body)
		}
	})

	// RFC 7644 Section 4: an HTTP GET to "/Schemas" SHALL return all supported schemas.
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
	// RFC 7643 Section 5: make the authenticationSchemes attribute publicly accessible without prior authentication.
	t.Run("authenticationSchemes is accessible without prior authentication", func(t *testing.T) {
		srv := newTestServer(t)

		response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/ServiceProviderConfig"))

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.NotEmpty(t, ReadBodyAs[core.ServiceProviderConfig](t, response).AuthenticationSchemes)
		assert.Equal(t, http.StatusUnauthorized, Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Schemas")).StatusCode)
	})

	// RFC 7644 Section 3.12: 501 when the service provider does not support the requested operation, e.g., PATCH.
	t.Run("PATCH is declined when Patch.Supported is false", func(t *testing.T) {
		config := core.NewServiceProviderConfig().Sorting().Filtering(protocol.DefaultLimits.MaxCount).Versioning()
		srv := newTestServer(t, withConfig(config))
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

	// RFC 7644 Section 3.12: 501 when the service provider does not support the requested operation.
	t.Run("filter is declined when Filter.Supported is false", func(t *testing.T) {
		srv := newTestServer(t, withConfig(core.NewServiceProviderConfig()))

		path := basePath + "/Users?" + url.Values{"filter": {`userName eq "bjensen"`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken))
		response := Response(t, srv, request)

		assert.Equal(t, http.StatusNotImplemented, response.StatusCode)
	})

	// RFC 7643 Section 5: filter.supported specifies whether or not the filter operation is supported.
	t.Run("plain listing still works when Filter.Supported is false", func(t *testing.T) {
		srv := newTestServer(t, withConfig(core.NewServiceProviderConfig()))

		request := Request(t, srv, http.MethodGet, basePath+"/Users", WithBearerToken(validToken))
		response := Response(t, srv, request)

		assert.Equal(t, http.StatusOK, response.StatusCode)
	})

	// RFC 7644 Section 3.12: 501 when the service provider does not support the requested operation.
	t.Run("sortBy is declined when Sort.Supported is false", func(t *testing.T) {
		srv := newTestServer(t, withConfig(core.NewServiceProviderConfig()))

		path := basePath + "/Users?" + url.Values{"sortBy": {"userName"}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken))
		response := Response(t, srv, request)

		assert.Equal(t, http.StatusNotImplemented, response.StatusCode)
	})

	// RFC 7644 Section 3.14: when supported, SCIM ETags MUST be specified as an HTTP header.
	t.Run("no ETag header when ETag.Supported is false, and a stale If-Match is ignored", func(t *testing.T) {
		config := core.NewServiceProviderConfig().Patching()
		srv := newTestServer(t, withConfig(config))

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

	// RFC 7643 Section 5: the configuration lets clients discover the SCIM specification features a service provider supports.
	t.Run("runs a minimal server with only CRUD end to end", func(t *testing.T) {
		srv := newTestServer(t, withConfig(core.NewServiceProviderConfig()))
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

// RFC 7644 2.2 Anonymous Requests
func TestRFC7644AnonymousRequests(t *testing.T) {
	t.Skip("MAY: anonymous requests are not supported; every request needs a bearer token")
}

// RFC 7644 3.3 Creating Resources
func TestRFC7644CreatingResources(t *testing.T) {
	// RFC 7644 Section 3.3: a successful create SHALL return 201 with the resource URI in the "Location" header.
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

	// RFC 7644 Section 3.12: invalidSyntax when the request body message structure was invalid.
	t.Run("rejects a malformed JSON body", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodPost, basePath+"/Users", WithBearerToken(validToken), WithRequestBody([]byte(`{`)))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		scimErr := ReadBodyAs[scimerrors.Error](t, response)
		assert.Equal(t, scimerrors.InvalidSyntax, scimErr.ScimType)
	})

	// RFC 7644 Section 3.12: invalidSyntax when the request body message structure was invalid.
	t.Run("rejects a body with data after the JSON value", func(t *testing.T) {
		srv := newTestServer(t)

		for _, body := range []string{`{"userName":"a"}garbage`, `{"userName":"b"}{"userName":"c"}`, `{"userName":"d"}]`} {
			request := Request(t, srv, http.MethodPost, basePath+"/Users", WithBearerToken(validToken), WithRequestBody([]byte(body)))
			response := Response(t, srv, request)

			require.Equal(t, http.StatusBadRequest, response.StatusCode, body)
			assert.Equal(t, scimerrors.InvalidSyntax, ReadBodyAs[scimerrors.Error](t, response).ScimType, body)
		}

		request := Request(t, srv, http.MethodPost, basePath+"/Users", WithBearerToken(validToken), WithRequestBody([]byte("{\"userName\":\"e\"}\r\n")))
		assert.Equal(t, http.StatusCreated, Response(t, srv, request).StatusCode)
	})

	// RFC 7644 Section 3.12: invalidSyntax when the request body did not conform to the request schema.
	t.Run("rejects a body that does not match the resource", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodPost, basePath+"/Users", WithBearerToken(validToken), WithRequestBody([]byte(`{"userName":5}`)))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidSyntax, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7644 Section 3.3: a create that conflicts with existing resources MUST return 409 with scimType "uniqueness".
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

	// RFC 7644 Section 3.3: a create that conflicts with existing resources MUST return 409 with scimType "uniqueness".
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

	// RFC 7643 Section 4.1.1: userName MUST be unique across the service provider's Users and is case insensitive.
	t.Run("rejects a unique value that differs only in case", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "Bjensen"})

		request := Request(t, srv, http.MethodPost, basePath+"/Users",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, core.User{UserName: "bjensen"}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusConflict, response.StatusCode)
		assert.Equal(t, scimerrors.Uniqueness, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7643 Section 7: a "server" unique value SHOULD be unique within the current SCIM endpoint.
	t.Run("frees a unique value that a replace gives up", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		response := Response(t, srv, Request(t, srv, http.MethodPut, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, core.User{UserName: "babs"}),
		))
		require.Equal(t, http.StatusOK, response.StatusCode)

		create(t, srv, &core.User{UserName: "bjensen"})
	})

	// RFC 7643 Section 7: a "server" unique value SHOULD be unique within the current SCIM endpoint.
	t.Run("frees a unique value when its resource is deleted", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		response := Response(t, srv, Request(t, srv, http.MethodDelete, basePath+"/Users/"+id, WithBearerToken(validToken)))
		require.Equal(t, http.StatusNoContent, response.StatusCode)

		create(t, srv, &core.User{UserName: "bjensen"})
	})

	// RFC 7643 Section 7: a "server" unique value SHOULD be unique within the current SCIM endpoint.
	t.Run("keeps a unique value after a failed replace", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})
		create(t, srv, &core.User{UserName: "babs"})

		response := Response(t, srv, Request(t, srv, http.MethodPut, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, core.User{UserName: "babs"}),
		))
		require.Equal(t, http.StatusConflict, response.StatusCode)

		request := Request(t, srv, http.MethodPost, basePath+"/Users",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, core.User{UserName: "bjensen"}),
		)
		assert.Equal(t, http.StatusConflict, Response(t, srv, request).StatusCode)
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

	// RFC 7643 Section 2.1: attribute names are case insensitive and the character set is US-ASCII.
	t.Run("rejects an attribute name that is not US-ASCII or repeats in another case", func(t *testing.T) {
		srv := newTestServer(t)
		extension := string(core.SchemaEnterpriseUser)

		for _, body := range []string{
			`{"userName":"alice","USERNAME":"bob"}`,
			`{"userName":"alice","` + extension + `":{"department":"ops"},"` + strings.ToLower(extension) + `":{"manager":{"displayName":"forged"}}}`,
			`{"userName":"alice","` + extension + `":{"manager":{"value":"m-1"},"MANAGER":{"displayName":"forged"}}}`,
			`{"userName":"alice","` + extension + `":{"department":"ops"},"` + strings.Replace(extension, "enterprise", "enterpri\u017fe", 1) + `":{"manager":{"displayName":"forged"}}}`,
			`{"userName":"alice","nick\u00e9":"bob"}`,
		} {
			request := Request(t, srv, http.MethodPost, basePath+"/Users",
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBody([]byte(body)),
			)
			response := Response(t, srv, request)

			require.Equal(t, http.StatusBadRequest, response.StatusCode, body)
			assert.Equal(t, scimerrors.InvalidSyntax, ReadBodyAs[scimerrors.Error](t, response).ScimType)
		}
	})

	// RFC 8259 Section 4: the names within an object SHOULD be unique.
	t.Run("rejects a duplicate attribute name", func(t *testing.T) {
		srv := newTestServer(t)

		response := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Users",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBody([]byte(`{"userName":"alice","userName":"bob"}`)),
		))

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidSyntax, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7644 Section 3.3: in the request body, attributes whose mutability is "readOnly" SHALL be ignored.
	t.Run("ignores readOnly groups", func(t *testing.T) {
		srv := newTestServer(t)

		id, _ := create(t, srv, &core.User{UserName: "alice", Groups: []core.GroupMembership{{Value: "g-1", Type: "direct"}}})

		response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users/"+id, WithBearerToken(validToken)))
		assert.NotContains(t, ReadBodyAs[map[string]any](t, response), "groups")
	})

	// RFC 7643 Section 7: a returned "request" attribute is returned in response to a PUT, POST, or PATCH that specified it.
	t.Run("returns a request attribute the client wrote", func(t *testing.T) {
		srv := newTestServer(t)
		id := createWidget(t, srv, &widget{Name: "gizmo"})["id"].(string)
		patchOf := func(op patch.Operation) protocol.PatchRequest {
			return protocol.PatchRequest{Schemas: []core.SchemaURI{protocol.SchemaPatchOp}, Operations: []patch.Operation{op}}
		}

		for _, tc := range []struct {
			method, path string
			body         any
			want         string
		}{
			{http.MethodPost, "/Widgets", &widget{Name: "one", Note: "n1"}, "n1"},
			{http.MethodPost, "/Widgets?attributes=name", &widget{Name: "two", Note: "n2"}, ""},
			{http.MethodPost, "/Widgets?excludedAttributes=note", &widget{Name: "three", Note: "n3"}, ""},
			{http.MethodPut, "/Widgets/" + id, &widget{Name: "gizmo", Note: "n4"}, "n4"},
			{http.MethodPatch, "/Widgets/" + id, patchOf(patch.Operation{Op: patch.OpReplace, Path: "note", Value: json.RawMessage(`"n5"`)}), "n5"},
			{http.MethodPatch, "/Widgets/" + id, patchOf(patch.Operation{Op: patch.OpReplace, Value: json.RawMessage(`{"note":"n6"}`)}), "n6"},
			{http.MethodPatch, "/Widgets/" + id, patchOf(patch.Operation{Op: patch.OpReplace, Path: "score", Value: json.RawMessage(`7`)}), ""},
		} {
			response := Response(t, srv, Request(t, srv, tc.method, basePath+tc.path, WithBearerToken(validToken), WithContentType(protocol.MediaType), WithRequestBodyAs(t, tc.body)))

			require.Less(t, response.StatusCode, http.StatusMultipleChoices, "%s %s", tc.method, tc.path)
			assert.Equal(t, tc.want, ReadBodyAs[widget](t, response).Note, "%s %s", tc.method, tc.path)
		}

		response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Widgets/"+id, WithBearerToken(validToken)))
		assert.Empty(t, ReadBodyAs[widget](t, response).Note)
	})

	// RFC 7643 Section 7: an attribute returned "never" is never returned, even when requested.
	t.Run("never returns the writeOnly password", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodPost, basePath+"/Users?attributes=password",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, core.User{UserName: "alice", Password: "t1meMa$heen"}),
		)
		response := Response(t, srv, request)
		require.Equal(t, http.StatusCreated, response.StatusCode)
		created := ReadBodyAs[map[string]any](t, response)
		assert.NotContains(t, created, "password")

		for _, query := range []string{"", "?attributes=password"} {
			response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users/"+created["id"].(string)+query, WithBearerToken(validToken)))
			assert.NotContains(t, ReadBodyAs[map[string]any](t, response), "password", query)
		}
	})

	// RFC 7643 Section 7: a "writeOnly" attribute's values SHALL NOT be returned.
	t.Run("never returns a writeOnly attribute", func(t *testing.T) {
		srv := newTestServer(t)
		created := createWidget(t, srv, &widget{Name: "gear", Secret: "wr1te-0nly"})

		for _, query := range []string{"", "?attributes=secret"} {
			response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Widgets/"+created["id"].(string)+query, WithBearerToken(validToken)))
			require.Equal(t, http.StatusOK, response.StatusCode)
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			assert.NotContains(t, string(body), "wr1te-0nly", query)
		}
	})

	// RFC 7644 Section 3.12: invalidSyntax when the request body message structure was invalid.
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

	// RFC 7644 Section 3.3: a create that conflicts with existing resources MUST return 409 with scimType "uniqueness".
	t.Run("admits one of many concurrent creates of a unique value", func(t *testing.T) {
		srv := newTestServer(t)

		requests := make([]*http.Request, 16)
		for i := range requests {
			requests[i] = Request(t, srv, http.MethodPost, basePath+"/Users",
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBodyAs(t, core.User{UserName: "bjensen"}),
			)
		}

		created, conflicted := 0, 0
		for _, status := range statusCodes(ConcurrentResponses(t, srv, requests...)) {
			switch status {
			case http.StatusCreated:
				created++
			case http.StatusConflict:
				conflicted++
			}
		}
		assert.Equal(t, 1, created)
		assert.Equal(t, len(requests)-1, conflicted)
	})
}

// RFC 7644 3.4.1 Retrieving a Known Resource
func TestRFC7644RetrievingAKnownResource(t *testing.T) {
	// RFC 7644 Section 3.4.1: if the resource exists, the server responds with 200 and includes the result in the body.
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

	// RFC 7644 Section 3.12: 404 when the specified resource does not exist.
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
	// RFC 7644 Section 3.4.2: totalResults is the total number of results returned by the list or query operation.
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
		require.Len(t, list.Resources, 3)
		assert.Equal(t, "alice", list.Resources[0].UserName)
		assert.Equal(t, []core.SchemaURI{core.SchemaUser}, list.Resources[0].Schemas)
		assert.NotEmpty(t, list.Resources[0].Meta.Location)
		assert.Equal(t, "bob", list.Resources[1].UserName)
		assert.Equal(t, "carol", list.Resources[2].UserName)
	})

	// RFC 7644 Section 3.4.2: a query returns zero or more resources, and Resources is REQUIRED only if totalResults is non-zero.
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

	// RFC 7644 Section 3.4.2: responses MUST use the ListResponse URI, and unrecognized query parameters SHOULD be ignored.
	t.Run("identifies the response with the ListResponse schema and ignores unknown parameters", func(t *testing.T) {
		srv := newTestServer(t)

		response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users?unknown=1", WithBearerToken(validToken)))

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, []core.SchemaURI{protocol.SchemaListResponse}, ReadBodyAs[protocol.ListResponse[map[string]any]](t, response).Schemas)
	})
}

// RFC 7644 3.4.2.1 Query Endpoints
func TestRFC7644QueryEndpoints(t *testing.T) {
	t.Skip("MAY: queries against the server root are not supported, so tooMany is never returned")
}

// RFC 7644 3.4.2.2 Filtering
func TestRFC7644Filtering(t *testing.T) {
	// RFC 7644 Section 3.4.2.2: filter=schemas eq "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User"
	t.Run("filters resources by schemas", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice", EnterpriseUser: &core.EnterpriseUser{EmployeeNumber: "1"}})
		create(t, srv, &core.User{UserName: "bob"})

		path := basePath + "/Users?" + url.Values{"filter": {`schemas eq "` + string(core.SchemaEnterpriseUser) + `"`}}.Encode()
		response := Response(t, srv, Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType)))

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Equal(t, 1, list.TotalResults)
		assert.Equal(t, "alice", list.Resources[0].UserName)
	})

	// RFC 7644 Section 3.4.2.2: when specified, only those resources matching the filter expression SHALL be returned.
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

	// RFC 7644 Section 3.12: invalidFilter when the attribute and filter comparison combination is not supported.
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

	// RFC 7644 Section 3.4.2.2: eq matches identical values and ne matches values that are not identical.
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

	// RFC 7643 Section 2.5: unassigned attributes and the null value SHALL be considered equivalent.
	t.Run("filters with eq null and ne null as unassigned and assigned, and ne a value as including unassigned", func(t *testing.T) {
		srv := newTestServer(t)
		active := true
		create(t, srv, &core.User{UserName: "alice", NickName: "al", Active: &active, Emails: []core.Email{{Value: "alice@example.com"}}})
		create(t, srv, &core.User{UserName: "bob"})

		for filter, want := range map[string]string{
			`nickName eq null`:                    "bob",
			`nickName ne null`:                    "alice",
			`active eq null`:                      "bob",
			`emails eq null`:                      "bob",
			`emails ne null`:                      "alice",
			`emails.value eq null`:                "bob",
			`nickName ne "al"`:                    "bob",
			`active ne true`:                      "bob",
			`emails.value ne "alice@example.com"`: "bob",
		} {
			path := basePath + "/Users?" + url.Values{"filter": {filter}}.Encode()
			request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
			response := Response(t, srv, request)

			require.Equal(t, http.StatusOK, response.StatusCode, filter)
			list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
			require.Equal(t, 1, list.TotalResults, filter)
			assert.Equal(t, want, list.Resources[0].UserName, filter)
		}
	})

	// RFC 7644 Section 3.4.2.2: pr matches an attribute with a non-empty or non-null value.
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

	// RFC 7644 Section 3.4.2.2: pr matches an attribute with a non-empty or non-null value.
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

	// RFC 7643 Section 3.1: "id" is a common attribute of every resource.
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

	// RFC 7644 Section 3.4.2.2: pr matches a complex attribute that contains a non-empty node.
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

	// RFC 7644 Section 3.4.2.2: for complex attributes, a fully qualified sub-attribute MUST be specified.
	t.Run("rejects a comparison on a complex attribute", func(t *testing.T) {
		srv := newTestServer(t)

		for _, filter := range []string{`name eq "x"`, `addresses eq "x"`} {
			path := basePath + "/Users?" + url.Values{"filter": {filter}}.Encode()
			response := Response(t, srv, Request(t, srv, http.MethodGet, path, WithBearerToken(validToken)))

			require.Equal(t, http.StatusBadRequest, response.StatusCode, filter)
			assert.Equal(t, scimerrors.InvalidFilter, ReadBodyAs[scimerrors.Error](t, response).ScimType, filter)
		}
	})

	// RFC 7644 Section 3.4.2.2: the expression within square brackets MUST be a valid filter expression based upon sub-attributes of the parent attribute.
	t.Run("rejects a value path filter whose inner expression is invalid", func(t *testing.T) {
		srv := newTestServer(t)

		path := basePath + "/Users?" + url.Values{"filter": {`emails[bogus eq "x"]`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidFilter, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7643 Section 3.1: "externalId" and "meta" are common attributes of every resource.
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

	// RFC 7644 Section 3.4.2.2: co, sw, and ew match a substring anywhere, at the start, and at the end of the value.
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

	// RFC 7644 Section 3.4.2.2: a bare multi-valued attribute name compares its "value" sub-attribute.
	t.Run("filters a multi-valued attribute name without a sub-attribute", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice", Emails: []core.Email{{Value: "alice@example.com"}}})
		create(t, srv, &core.User{UserName: "bob", Emails: []core.Email{{Value: "bob@example.org"}}})

		path := basePath + "/Users?" + url.Values{"filter": {`emails co "example.com"`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Equal(t, 1, list.TotalResults)
		assert.Equal(t, "alice", list.Resources[0].UserName)
	})

	// RFC 7644 Section 3.4.2.2: and is only a match if both expressions evaluate to true.
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

	// RFC 7644 Section 3.4.2.2: or is a match if either expression evaluates to true.
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

	// RFC 7644 Section 3.4.2.2: not is a match if the expression evaluates to false.
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

	// RFC 7644 Section 3.4.2.2: for string attribute types, gt, ge, lt, and le are a lexicographical comparison.
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

	// RFC 7644 Section 3.4.2.2: valuePath = attrPath "[" valFilter "]"
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

	// RFC 7644 Section 3.4.2.2: complex attribute filter expressions MUST be applied to the same value of a parent attribute.
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

	// RFC 7644 Section 3.4.2.2: valFilter = attrExp / logExp / *1"not" "(" valFilter ")"
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

	// RFC 7644 Section 3.12: invalidFilter when the attribute and filter comparison combination is not supported.
	t.Run("rejects a value path filter on a single-valued complex attribute", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice", Name: core.Name{GivenName: "alice"}})

		path := basePath + "/Users?" + url.Values{"filter": {`name[givenName eq "alice"]`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidFilter, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7644 Section 3.4.2.2: FILTER = attrExp / logExp / valuePath / *1"not" "(" FILTER ")"
	t.Run("rejects a sub-attribute after a value path filter", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice", Emails: []core.Email{{Value: "a@work.com", Type: "work"}}})

		path := basePath + "/Users?" + url.Values{"filter": {`emails[type eq "work"].value`}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidFilter, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7644 Section 3.4.2.2: valFilter = attrExp / logExp / *1"not" "(" valFilter ")"
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

	// RFC 7644 Section 3.4.2.2: attrExp = (attrPath SP "pr") / (attrPath SP compareOp SP compValue)
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

	// RFC 7644 Section 3.4.2.2: for integer attributes, gt, ge, lt, and le are a comparison by numeric value.
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

	// RFC 7644 Section 3.4.2.2: for DateTime types, gt, ge, lt, and le are a chronological comparison.
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

	// RFC 7643 Section 2.5: unassigned attributes and the null value SHALL be considered equivalent.
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

	// RFC 7644 Section 3.4.2.2: the actual comparison is dependent on the attribute type.
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

	// RFC 7644 Section 3.4.2.2: gt, ge, lt, and le on Boolean and Binary attributes SHALL cause a failed response with "invalidFilter".
	t.Run("orders every attribute type except boolean and binary", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice", ProfileURL: "https://a.example.com"})
		create(t, srv, &core.User{UserName: "bob", ProfileURL: "https://b.example.com"})
		create(t, srv, &core.User{UserName: "carol"})

		search := func(filter string) *http.Response {
			path := basePath + "/Users?" + url.Values{"filter": {filter}}.Encode()
			return Response(t, srv, Request(t, srv, http.MethodGet, path, WithBearerToken(validToken)))
		}
		userNames := func(filter string) []string {
			response := search(filter)
			require.Equal(t, http.StatusOK, response.StatusCode, filter)
			users := ReadBodyAs[protocol.ListResponse[*core.User]](t, response).Resources
			names := make([]string, 0, len(users))
			for _, user := range users {
				names = append(names, user.UserName)
			}
			return names
		}

		assert.Equal(t, []string{"bob"}, userNames(`profileUrl gt "https://a.example.com"`))
		assert.Equal(t, []string{"alice"}, userNames(`profileUrl lt "https://b.example.com"`))
		for _, filter := range []string{`active gt true`, `x509Certificates.value gt "x"`} {
			response := search(filter)
			require.Equal(t, http.StatusBadRequest, response.StatusCode, filter)
			assert.Equal(t, scimerrors.InvalidFilter, ReadBodyAs[scimerrors.Error](t, response).ScimType, filter)
		}
	})

	// RFC 7644 Section 3.4.2.2: attribute names and attribute operators used in filters are case insensitive.
	t.Run("treats attribute names and operators as case insensitive", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "bjensen"})

		path := basePath + "/Users?" + url.Values{"filter": {`USERNAME EQ "bjensen"`}}.Encode()
		response := Response(t, srv, Request(t, srv, http.MethodGet, path, WithBearerToken(validToken)))

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, 1, ReadBodyAs[protocol.ListResponse[map[string]any]](t, response).TotalResults)
	})

	// RFC 7644 Section 3.4.2.2: a URI-qualified attribute name reaches into a schema extension.
	t.Run("filters by a URI-qualified extension attribute", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen", EnterpriseUser: &core.EnterpriseUser{EmployeeNumber: "1234"}})

		filterQuery := string(core.SchemaEnterpriseUser) + `:employeeNumber eq "1234"`
		path := basePath + "/Users?" + url.Values{"filter": {filterQuery}}.Encode()
		response := Response(t, srv, Request(t, srv, http.MethodGet, path, WithBearerToken(validToken)))

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Equal(t, 1, list.TotalResults)
		assert.Equal(t, id, list.Resources[0].ID)
	})

	// RFC 7644 Section 3.10: every facet of an attribute path, including the URN, is case insensitive.
	t.Run("filters by an extension attribute with an upper-case URN", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen", EnterpriseUser: &core.EnterpriseUser{EmployeeNumber: "1234"}})

		filterQuery := strings.ToUpper(string(core.SchemaEnterpriseUser)) + `:employeeNumber eq "1234"`
		path := basePath + "/Users?" + url.Values{"filter": {filterQuery}}.Encode()
		response := Response(t, srv, Request(t, srv, http.MethodGet, path, WithBearerToken(validToken)))

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Equal(t, 1, list.TotalResults)
		assert.Equal(t, id, list.Resources[0].ID)
	})
}

// RFC 7644 3.4.2.3 Sorting
func TestRFC7644Sorting(t *testing.T) {
	// RFC 7644 Section 3.4.2.3: if "sortBy" is provided and no "sortOrder" is specified, "sortOrder" SHALL default to ascending.
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

	// RFC 7644 Section 3.4.2.3: allowed "sortOrder" values are "ascending" and "descending".
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

	// RFC 7644 Section 3.4.2.3: "sortBy" specifies the attribute whose value SHALL be used to order the returned responses.
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

	// RFC 7644 Section 3.4.2.3: for a complex attribute, "sortBy" must be a path to a sub-attribute, e.g., "name.givenName".
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

	// RFC 7644 Section 3.4.2.3: resources with no data for "sortBy" are ordered last if ascending and first if descending.
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

	// RFC 7644 Section 3.12: invalidValue when the value specified was not compatible with the operation or attribute type.
	t.Run("rejects sortBy on an attribute that is not known", func(t *testing.T) {
		srv := newTestServer(t)

		path := basePath + "/Users?" + url.Values{"sortBy": {"bogus"}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidValue, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7643 Section 7: "writeOnly" attribute values SHALL NOT be returned.
	t.Run("rejects sortBy on a writeOnly attribute", func(t *testing.T) {
		srv := newTestServer(t)

		path := basePath + "/Users?" + url.Values{"sortBy": {"password"}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidValue, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7643 Section 7: "writeOnly" attribute values SHALL NOT be returned.
	t.Run("rejects sortBy on a sub-attribute of a writeOnly attribute", func(t *testing.T) {
		srv := newTestServer(t)

		path := basePath + "/Widgets?" + url.Values{"sortBy": {"keys.value"}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidValue, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7644 Section 3.4.2.3: "sortBy" uses standard attribute notation (Section 3.10).
	t.Run("rejects sortBy with an invalid attribute path syntax", func(t *testing.T) {
		srv := newTestServer(t)

		path := basePath + "/Users?" + url.Values{"sortBy": {"1bad"}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidValue, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7644 Section 3.4.2.3: "sortOrder" MUST sort according to the attribute type.
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

	// RFC 7644 Section 3.4.2.3: "sortOrder" MUST sort according to the attribute type.
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

	// RFC 7644 Section 3.4.2.3: "sortOrder" MUST sort according to the attribute type.
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

	// RFC 7644 Section 3.4.2.3: a multi-valued attribute without a sub-attribute sorts by its primary value, or else its first value.
	t.Run("sorts by a multi-valued attribute name without a sub-attribute", func(t *testing.T) {
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

		path := basePath + "/Users?" + url.Values{"sortBy": {"emails"}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Len(t, list.Resources, 3)
		assert.Equal(t, "mia", list.Resources[0].UserName)
		assert.Equal(t, "zed", list.Resources[1].UserName)
		assert.Equal(t, "nobody", list.Resources[2].UserName)
	})

	// RFC 7644 Section 3.4.2.3: allowed "sortOrder" values are "ascending" and "descending".
	t.Run("rejects a sortOrder that is not ascending or descending", func(t *testing.T) {
		srv := newTestServer(t)

		response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users?sortBy=userName&sortOrder=sideways", WithBearerToken(validToken)))

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidValue, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7644 Section 3.4.2.3: sortBy also reaches into a schema extension.
	t.Run("sorts by a URI-qualified extension attribute", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "bjensen", EnterpriseUser: &core.EnterpriseUser{EmployeeNumber: "1234"}})
		create(t, srv, &core.User{UserName: "amorris", EnterpriseUser: &core.EnterpriseUser{EmployeeNumber: "0001"}})

		sortBy := string(core.SchemaEnterpriseUser) + ":employeeNumber"
		path := basePath + "/Users?" + url.Values{"sortBy": {sortBy}}.Encode()
		response := Response(t, srv, Request(t, srv, http.MethodGet, path, WithBearerToken(validToken)))

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Len(t, list.Resources, 2)
		assert.Equal(t, "amorris", list.Resources[0].UserName)
	})

	// RFC 7644 Section 3.10: every facet of an attribute path, including the URN, is case insensitive.
	t.Run("sorts by an extension attribute with an upper-case URN", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "bjensen", EnterpriseUser: &core.EnterpriseUser{EmployeeNumber: "1234"}})
		create(t, srv, &core.User{UserName: "amorris", EnterpriseUser: &core.EnterpriseUser{EmployeeNumber: "0001"}})

		sortBy := strings.ToUpper(string(core.SchemaEnterpriseUser)) + ":employeeNumber"
		path := basePath + "/Users?" + url.Values{"sortBy": {sortBy}}.Encode()
		response := Response(t, srv, Request(t, srv, http.MethodGet, path, WithBearerToken(validToken)))

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
		require.Len(t, list.Resources, 2)
		assert.Equal(t, "amorris", list.Resources[0].UserName)
	})
}

// RFC 7644 3.4.2.4 Pagination
func TestRFC7644Pagination(t *testing.T) {
	// RFC 7644 Section 3.4.2.4: startIndex is the 1-based index of the first query result and count is the maximum per page.
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

	// RFC 7644 Section 3.4.2.4: if count is unspecified, the maximum number of results is set by the service provider.
	t.Run("pages with a custom default count", func(t *testing.T) {
		srv := newTestServer(t, withOption(server.DefaultCount(1)))
		create(t, srv, &core.User{UserName: "alice"})
		create(t, srv, &core.User{UserName: "bob"})

		request := Request(t, srv, http.MethodGet, basePath+"/Users", WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, 1, ReadBodyAs[protocol.ListResponse[*core.User]](t, response).ItemsPerPage)
	})

	// RFC 7644 Section 3.4.2.4: the service provider MUST NOT return more results than specified, although it MAY return fewer.
	t.Run("caps the count at a custom maximum", func(t *testing.T) {
		config := core.NewServiceProviderConfig().Sorting().Filtering(2).Patching().Versioning()
		srv := newTestServer(t, withConfig(config))
		create(t, srv, &core.User{UserName: "alice"})
		create(t, srv, &core.User{UserName: "bob"})
		create(t, srv, &core.User{UserName: "carol"})

		path := basePath + "/Users?" + url.Values{"count": {"50"}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, 2, ReadBodyAs[protocol.ListResponse[*core.User]](t, response).ItemsPerPage)
	})

	// RFC 7644 Section 3.12: invalidValue when the value specified was not compatible with the operation or attribute type.
	t.Run("rejects a query with a non-integer startIndex", func(t *testing.T) {
		srv := newTestServer(t)

		path := basePath + "/Users?" + url.Values{"startIndex": {"bogus"}}.Encode()
		request := Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidValue, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7644 Section 3.4.2.4: a startIndex less than 1 SHALL be interpreted as 1 and a negative count as "0".
	t.Run("reads a startIndex below 1 as 1 and a negative count as 0", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "bjensen"})
		create(t, srv, &core.User{UserName: "jsmith"})
		list := func(query string) protocol.ListResponse[map[string]any] {
			response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users?"+query, WithBearerToken(validToken)))
			require.Equal(t, http.StatusOK, response.StatusCode, query)
			return ReadBodyAs[protocol.ListResponse[map[string]any]](t, response)
		}

		assert.Equal(t, 1, list("startIndex=0&count=1").StartIndex)
		assert.Empty(t, list("count=-1").Resources)
		zero := list("count=0")
		assert.Empty(t, zero.Resources)
		assert.Equal(t, 2, zero.TotalResults)
	})
}

// RFC 7644 3.4.2.5 Attributes
func TestRFC7644Attributes(t *testing.T) {
	srv := newTestServer(t)
	id, _ := create(t, srv, &core.User{
		UserName:       "bjensen",
		Emails:         []core.Email{{Value: "bjensen@example.com", Type: "work"}},
		EnterpriseUser: &core.EnterpriseUser{Department: "Tour Operations", CostCenter: "4130"},
	})
	get := func(t *testing.T, path string) (int, map[string]any) {
		t.Helper()
		response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+path, WithBearerToken(validToken)))
		return response.StatusCode, ReadBodyAs[map[string]any](t, response)
	}

	// RFC 7644 Section 3.9: each resource MUST contain the minimum set and any attributes explicitly requested by "attributes".
	t.Run("lists only the minimum set and the requested attributes", func(t *testing.T) {
		status, body := get(t, "/Users?attributes=userName")
		require.Equal(t, http.StatusOK, status)
		resources := body["Resources"].([]any)
		require.Len(t, resources, 1)
		assert.ElementsMatch(t, []string{"schemas", "id", "userName"}, keysOf(resources[0].(map[string]any)))
	})

	// RFC 7644 Section 3.9: each resource MUST contain the minimum set and any attributes explicitly requested by "attributes".
	t.Run("ignores requested attributes the schema does not declare", func(t *testing.T) {
		_, body := get(t, "/Users/"+id+"?attributes=bogus")
		assert.ElementsMatch(t, []string{"schemas", "id"}, keysOf(body))

		_, body = get(t, "/Users/"+id+"?attributes=bogus,userName,userName")
		assert.ElementsMatch(t, []string{"schemas", "id", "userName"}, keysOf(body))

		_, body = get(t, "/Users/"+id+"?excludedAttributes=bogus")
		assert.Equal(t, "bjensen", body["userName"])
	})

	// RFC 7644 Section 3.9: excludedAttributes returns the minimum set plus the default set minus the excluded attributes.
	t.Run("fetches a resource without the excluded attributes", func(t *testing.T) {
		status, body := get(t, "/Users/"+id+"?excludedAttributes=emails,meta")
		require.Equal(t, http.StatusOK, status)
		assert.Equal(t, "bjensen", body["userName"])
		assert.NotContains(t, body, "emails")
		assert.NotContains(t, body, "meta")
	})

	// RFC 7644 Section 3.9: the attributes returned are the minimum attribute set plus the default attribute set.
	t.Run("omits attributes the schema does not declare", func(t *testing.T) {
		_, body := get(t, "/Users/"+id)
		enterprise := body[string(core.SchemaEnterpriseUser)].(map[string]any)
		assert.Equal(t, "Tour Operations", enterprise["department"])
		assert.NotContains(t, enterprise, "costCenter")
	})

	// RFC 7644 Section 3.9: "attributes" and "excludedAttributes" are mutually exclusive.
	t.Run("rejects attributes together with excludedAttributes", func(t *testing.T) {
		status, _ := get(t, "/Users/"+id+"?attributes=userName&excludedAttributes=emails")
		assert.Equal(t, http.StatusBadRequest, status)
	})

	// RFC 7644 Section 3.9: "attributes" and "excludedAttributes" are mutually exclusive.
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

	// RFC 7644 Section 3.9: clients MAY request a partial representation on any operation that returns a resource.
	t.Run("shapes the resource returned by a create", func(t *testing.T) {
		response := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Users?attributes=userName",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, core.User{UserName: "alice", Emails: []core.Email{{Value: "a@example.com"}}}),
		))
		require.Equal(t, http.StatusCreated, response.StatusCode)
		assert.ElementsMatch(t, []string{"schemas", "id", "userName"}, keysOf(ReadBodyAs[map[string]any](t, response)))
	})

	// RFC 7644 Section 3.9: clients MAY request a partial representation on any operation that returns a resource.
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

	// RFC 7644 Section 3.9: with excludedAttributes, each resource returned MUST contain the minimum set of attributes.
	t.Run("never excludes an attribute that is always returned", func(t *testing.T) {
		status, body := get(t, "/Users/"+id+"?excludedAttributes=id")

		require.Equal(t, http.StatusOK, status)
		assert.Equal(t, id, body["id"])
	})

	// RFC 7643 Section 7: an "always" attribute is returned regardless of the "attributes" parameter.
	t.Run("returns an always sub-attribute of a parent left out", func(t *testing.T) {
		created := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Gadgets",
			WithBearerToken(validToken), WithContentType(protocol.MediaType),
			WithRequestBody([]byte(`{"parts":[{"serial":"s-1","code":"A"}]}`)),
		))
		require.Equal(t, http.StatusCreated, created.StatusCode)
		gadget, _ := ReadBodyAs[map[string]any](t, created)["id"].(string)

		for _, query := range []string{"excludedAttributes=parts", "attributes=id"} {
			status, body := get(t, "/Gadgets/"+gadget+"?"+query)

			require.Equal(t, http.StatusOK, status, query)
			assert.Equal(t, []any{map[string]any{"Serial": "s-1"}}, body["Parts"], query)
		}
	})

	// RFC 7643 Section 2.5: unassigned attributes MAY be omitted.
	t.Run("omits elements that have none of the requested sub-attributes", func(t *testing.T) {
		_, body := get(t, "/Users/"+id+"?attributes=emails.display")
		assert.NotContains(t, body, "emails")

		_, body = get(t, "/Users/"+id+"?attributes=emails.value")
		assert.Equal(t, []any{map[string]any{"value": "bjensen@example.com"}}, body["emails"])
	})

	// RFC 7644 Section 3.9: clients MAY request a partial representation on any operation that returns a resource.
	t.Run("shapes the resource returned by a replace", func(t *testing.T) {
		response := Response(t, srv, Request(t, srv, http.MethodPut, basePath+"/Users/"+id+"?attributes=userName",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, core.User{UserName: "bjensen", Emails: []core.Email{{Value: "bjensen@example.com"}}}),
		))
		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.ElementsMatch(t, []string{"schemas", "id", "userName"}, keysOf(ReadBodyAs[map[string]any](t, response)))
	})

	// RFC 7644 Section 3.9: attributes and excludedAttributes shape the resource representation that is returned.
	t.Run("hands the repository the projection of a read and of a write's result", func(t *testing.T) {
		schemas := core.Schemas{core.NewSchema(core.SchemaUser).With(userAttributes()...)}
		repository := &projectingRepository[*core.User]{Repository: server.NewRepository[*core.User](basePath+"/Users", schemas), attribute: "emails"}
		srv := Server(t, server.New(basePath, fullServiceProviderConfig(),
			server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, userAttributes()...).WithRepository(repository)),
		))
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users/"+id+"?excludedAttributes=emails"))
		Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users?excludedAttributes=emails"))
		require.Equal(t, []bool{false, false}, repository.reads)

		response := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Users/"+id+"?excludedAttributes=emails",
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{{Op: patch.OpAdd, Path: "emails", Value: json.RawMessage(`[{"value":"b@example.com"}]`)}},
			}),
		))

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, []bool{false, false, true}, repository.reads)
		assert.Equal(t, []bool{true, false}, repository.writes)
	})

	// RFC 7644 Section 3.5.2: the server MAY return 204 (No Content) and the appropriate response headers.
	t.Run("hands the repository a write projection without members when a group patch answers 204", func(t *testing.T) {
		repository := &projectingRepository[*core.Group]{Repository: server.NewRepository[*core.Group](basePath+"/Groups", groupSchemas()), attribute: "members"}
		srv := Server(t, newGroupHandler(t, repository))
		created := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Groups", WithBearerToken(validToken), WithContentType(protocol.MediaType), WithRequestBodyAs(t, core.Group{DisplayName: "eng"})))
		require.Equal(t, http.StatusCreated, created.StatusCode)
		id := ReadBodyAs[core.Group](t, created).ID

		response := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Groups/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{{Op: patch.OpAdd, Path: "members", Value: json.RawMessage(`[{"value":"u-1","type":"User"}]`)}},
			}),
		))

		require.Equal(t, http.StatusNoContent, response.StatusCode)
		assert.Equal(t, []bool{true}, repository.reads)
		assert.Equal(t, []bool{true, false}, repository.writes)
		assert.True(t, repository.updated.Returns("id"))
		assert.True(t, repository.updated.Returns("meta"))
	})
}

// RFC 7644 3.4.3 Querying Resources Using HTTP POST
func TestRFC7644QueryingResourcesUsingHTTPPOST(t *testing.T) {
	search := func(t *testing.T, srv *httptest.Server, path, body string) *http.Response {
		t.Helper()
		return Response(t, srv, Request(t, srv, http.MethodPost, path,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBody([]byte(body)),
		))
	}

	// RFC 7644 Section 3.12: 501 when the service provider does not support the requested operation.
	t.Run("declines a search from the root", func(t *testing.T) {
		srv := newTestServer(t)

		response := search(t, srv, basePath+"/.search", `{"schemas":["urn:ietf:params:scim:api:messages:2.0:SearchRequest"]}`)

		assert.Equal(t, http.StatusNotImplemented, response.StatusCode)
		assert.Equal(t, "501", ReadBodyAs[scimerrors.Error](t, response).Status)
	})

	// RFC 7644 Section 3.4.3: after receiving an HTTP POST request, a response is returned as specified in Section 3.4.2.
	t.Run("answers a search on a resource endpoint with a ListResponse", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice"})
		create(t, srv, &core.User{UserName: "bjensen"})
		create(t, srv, &core.User{UserName: "bob"})

		response := search(t, srv, basePath+"/Users/.search", `{
			"schemas": ["urn:ietf:params:scim:api:messages:2.0:SearchRequest"],
			"attributes": ["userName"],
			"filter": "userName sw \"b\"",
			"sortBy": "userName",
			"sortOrder": "descending",
			"startIndex": 1,
			"count": 1
		}`)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[map[string]any]](t, response)
		assert.Equal(t, []core.SchemaURI{protocol.SchemaListResponse}, list.Schemas)
		assert.Equal(t, 2, list.TotalResults)
		require.Len(t, list.Resources, 1)
		assert.Equal(t, "bob", list.Resources[0]["userName"])
		assert.NotContains(t, list.Resources[0], "meta")
	})

	// RFC 7644 Section 3.4.3: clients MAY execute queries by using HTTP POST on the "/.search" path of any resource endpoint.
	t.Run("answers a search on the Groups endpoint", func(t *testing.T) {
		srv := newTestServer(t)
		for _, name := range []string{"Admins", "Tour Guides"} {
			createGroup(t, srv, &core.Group{DisplayName: name})
		}

		response := search(t, srv, basePath+"/Groups/.search", `{
			"schemas": ["urn:ietf:params:scim:api:messages:2.0:SearchRequest"],
			"filter": "displayName eq \"Admins\""
		}`)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[core.Group]](t, response)
		assert.Equal(t, 1, list.TotalResults)
		require.Len(t, list.Resources, 1)
		assert.Equal(t, "Admins", list.Resources[0].DisplayName)
	})

	// RFC 7644 Section 3.12: invalidValue when a request attribute has an incompatible value.
	t.Run("rejects an unknown attribute name", func(t *testing.T) {
		srv := newTestServer(t)

		response := search(t, srv, basePath+"/Users/.search", `{
			"schemas": ["urn:ietf:params:scim:api:messages:2.0:SearchRequest"],
			"attributes": ["leak!"]
		}`)

		assert.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidValue, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7644 Section 3.4.3: query requests MUST be identified using the SearchRequest URI.
	t.Run("rejects a search without the SearchRequest schema", func(t *testing.T) {
		srv := newTestServer(t)

		response := search(t, srv, basePath+"/Users/.search", `{"filter":"userName eq \"bjensen\""}`)

		assert.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidSyntax, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7644 Section 3.12: 501 when the service provider does not support the requested operation.
	t.Run("declines a filter when Filter.Supported is false", func(t *testing.T) {
		srv := newTestServer(t, withConfig(core.NewServiceProviderConfig()))

		response := search(t, srv, basePath+"/Users/.search", `{
			"schemas": ["urn:ietf:params:scim:api:messages:2.0:SearchRequest"],
			"filter": "userName eq \"bjensen\""
		}`)

		assert.Equal(t, http.StatusNotImplemented, response.StatusCode)
	})

	// RFC 7644 Section 3.12: 501 when the service provider does not support the requested operation.
	t.Run("declines sortBy when Sort.Supported is false", func(t *testing.T) {
		srv := newTestServer(t, withConfig(core.NewServiceProviderConfig()))

		response := search(t, srv, basePath+"/Users/.search", `{
			"schemas": ["urn:ietf:params:scim:api:messages:2.0:SearchRequest"],
			"sortBy": "userName"
		}`)

		assert.Equal(t, http.StatusNotImplemented, response.StatusCode)
	})
}

// RFC 7644 3.5.1 Replacing with PUT
func TestRFC7644ReplacingWithPUT(t *testing.T) {
	// RFC 7644 Section 3.14: the PUT succeeds only if the supplied If-Match ETag matches the latest resource.
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

	// RFC 7644 Section 3.14: ETags ensure that clients do not inadvertently overwrite each other's changes.
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

	// RFC 7644 Section 3.14: the client MAY supply an If-Match header for PUT.
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

	// RFC 7644 Section 3.12: 412 when the update failed because the resource has changed on the server.
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

	// RFC 9110 Section 13.1.1: the condition holds if any entity-tag in the If-Match list matches.
	t.Run("matches any ETag in an If-Match list", func(t *testing.T) {
		for _, test := range []struct {
			name   string
			lines  func(etag string) []string
			status int
		}{
			{"one line", func(etag string) []string { return []string{`W/"stale", ` + etag} }, http.StatusOK},
			{"two lines", func(etag string) []string { return []string{`W/"stale"`, etag} }, http.StatusOK},
			{"all stale", func(string) []string { return []string{`W/"a,b", W/"stale"`} }, http.StatusPreconditionFailed},
			{"malformed", func(string) []string { return []string{`"open, bare`} }, http.StatusPreconditionFailed},
			{"only separators", func(string) []string { return []string{` , `} }, http.StatusPreconditionFailed},
			{"empty lines", func(string) []string { return []string{"", ""} }, http.StatusPreconditionFailed},
		} {
			srv := newTestServer(t)
			id, etag := create(t, srv, &core.User{UserName: "bjensen"})
			ifMatch := func(r *http.Request) *http.Request {
				for _, line := range test.lines(etag) {
					r.Header.Add("If-Match", line)
				}
				return r
			}

			request := Request(t, srv, http.MethodPut, basePath+"/Users/"+id,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				ifMatch,
				WithRequestBody([]byte(`{"userName":"bjensen2"}`)),
			)
			response := Response(t, srv, request)

			assert.Equal(t, test.status, response.StatusCode, test.name)
		}
	})

	// RFC 7644 Section 3.12: 404 when the specified resource does not exist.
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

	// RFC 7644 Section 3.12: invalidSyntax when the request body message structure was invalid.
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

	// RFC 7644 Section 3.12: invalidSyntax when the request body message structure was invalid.
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

	// RFC 7644 Section 3.5.1: if an immutable value is already set, the input value(s) MUST match, or 400 SHOULD be returned with scimType "mutability".
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

	// RFC 7644 Section 3.5.1: if the service provider has no existing immutable values, the new value(s) SHALL be applied.
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

	// RFC 7644 Section 3.5.1: if the service provider has no existing immutable values, the new value(s) SHALL be applied.
	t.Run("allows assigning an immutable sub-attribute of a group member for the first time", func(t *testing.T) {
		srv := newTestServer(t)
		id := createGroup(t, srv, &core.Group{DisplayName: "eng", Members: []core.Member{{Value: "u-1"}}}).ID

		response := Response(t, srv, Request(t, srv, http.MethodPut, basePath+"/Groups/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, core.Group{DisplayName: "eng", Members: []core.Member{{Value: "u-1", Type: "User"}}}),
		))

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, []core.Member{{Value: "u-1", Type: "User"}}, ReadBodyAs[core.Group](t, response).Members)
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

	// RFC 7643 Section 7: a readOnly attribute SHALL NOT be modified.
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

	// RFC 7643 Section 7: a readOnly attribute SHALL NOT be modified.
	t.Run("keeps readOnly sub-attributes of the elements it resends", func(t *testing.T) {
		srv := newTestServer(t)
		id := createWidget(t, srv, &widget{Name: "gizmo", Parts: []part{{Serial: "s-1", Code: "c0de"}, {Serial: "s-2"}}})["id"].(string)

		request := Request(t, srv, http.MethodPut, basePath+"/Widgets/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBody([]byte(`{"name":"gizmo","parts":[{"serial":"s-1","inspector":"mallory"},{"serial":"s-3"}]}`)),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, []part{{Serial: "s-1", Inspector: "qa-bot"}, {Serial: "s-3"}}, ReadBodyAs[widget](t, response).Parts)
	})

	// RFC 7644 Section 3.5.1: if an attribute is "required", clients MUST specify the attribute in the PUT request.
	t.Run("rejects a replace missing a required attribute", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		response := Response(t, srv, Request(t, srv, http.MethodPut, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBody([]byte(`{}`)),
		))

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidValue, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7643 Section 3.1: lastModified is the most recent DateTime that the details of this resource were updated.
	t.Run("advances meta.lastModified", func(t *testing.T) {
		srv := newTestServer(t)
		created := ReadBodyAs[core.User](t, Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Users",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, core.User{UserName: "bjensen"}),
		)))

		response := Response(t, srv, Request(t, srv, http.MethodPut, basePath+"/Users/"+created.ID,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, core.User{UserName: "bjensen"}),
		))

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.True(t, ReadBodyAs[core.User](t, response).Meta.LastModified.After(created.Meta.LastModified))
	})

	// RFC 7644 Section 3.5.1: if an immutable value is already set, the input value(s) MUST match.
	t.Run("keeps an omitted immutable sub-attribute", func(t *testing.T) {
		srv := newTestServer(t)
		id, etag := create(t, srv, &core.User{UserName: "bjensen", Name: core.Name{GivenName: "Barbara", FamilyName: "Jensen"}})

		response := Response(t, srv, Request(t, srv, http.MethodPut, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithHeader("If-Match", etag),
			WithRequestBodyAs(t, core.User{UserName: "bjensen", Name: core.Name{GivenName: "Babs"}}),
		))

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, "Jensen", ReadBodyAs[core.User](t, response).Name.FamilyName)
	})

	// RFC 7644 Section 3.5.1: if an immutable value is already set, the input value(s) MUST match, or 400 SHOULD be returned with scimType "mutability".
	t.Run("group member mutability", func(t *testing.T) {
		seed := core.Member{Value: "u-1", Type: "User", Ref: "https://example.com/v2/Users/u-1"}
		for _, test := range []struct {
			name   string
			member core.Member
		}{
			{"allows an omitted type", core.Member{Value: "u-1", Ref: seed.Ref}},
			{"allows an omitted $ref", core.Member{Value: "u-1", Type: "User"}},
			{"accepts a new member list", core.Member{Value: "u-2", Type: "User"}},
		} {
			t.Run(test.name, func(t *testing.T) {
				srv := newTestServer(t)
				id := createGroup(t, srv, &core.Group{DisplayName: "eng", Members: []core.Member{seed}}).ID

				response := Response(t, srv, Request(t, srv, http.MethodPut, basePath+"/Groups/"+id,
					WithBearerToken(validToken),
					WithContentType(protocol.MediaType),
					WithRequestBodyAs(t, core.Group{DisplayName: "eng", Members: []core.Member{test.member}}),
				))

				require.Equal(t, http.StatusOK, response.StatusCode)
			})
		}

		for _, name := range []string{"rejects a changed type", "rejects a changed type when value differs only by case", "rejects a changed $ref"} {
			t.Run(name, func(t *testing.T) {
				t.Skip("MUST: member sub-attribute immutability is left to the Group repository, so the default server accepts the change")
			})
		}
	})

	// RFC 7643 Section 4.2: sub-attributes of group members are "immutable".
	t.Run("group member replace is idempotent", func(t *testing.T) {
		t.Run("PUT re-sending an existing member by value alone keeps its stored type", func(t *testing.T) {
			srv := newTestServer(t)
			id := createGroup(t, srv, &core.Group{DisplayName: "eng", Members: []core.Member{{Value: "u-1", Type: "User"}}}).ID

			response := Response(t, srv, Request(t, srv, http.MethodPut, basePath+"/Groups/"+id,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBodyAs(t, core.Group{DisplayName: "eng", Members: []core.Member{{Value: "u-1"}}}),
			))

			require.Equal(t, http.StatusOK, response.StatusCode)
			assert.Equal(t, []core.Member{{Value: "u-1", Type: "User"}}, ReadBodyAs[core.Group](t, response).Members)
		})

		t.Run("PATCH replace members with an already-present value and no type keeps the stored type", func(t *testing.T) {
			srv := newTestServer(t)
			id := createGroup(t, srv, &core.Group{DisplayName: "eng", Members: []core.Member{{Value: "u-1", Type: "User"}}}).ID

			response := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Groups/"+id,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBodyAs(t, protocol.PatchRequest{
					Schemas: []core.SchemaURI{protocol.SchemaPatchOp},
					Operations: []patch.Operation{
						{Op: patch.OpReplace, Path: "members", Value: json.RawMessage(`[{"value":"u-1"}]`)},
					},
				}),
			))

			require.Equal(t, http.StatusOK, response.StatusCode)
			assert.Equal(t, []core.Member{{Value: "u-1", Type: "User"}}, ReadBodyAs[core.Group](t, response).Members)
		})

		t.Run("a resend that omits type does not waive mutability for a later request", func(t *testing.T) {
			t.Skip("MUST: member sub-attribute immutability is left to the Group repository, so a later changed type is accepted")
		})

		t.Run("PATCH remove of an immutable sub-attribute of a matched member is still a mutability error", func(t *testing.T) {
			srv := newTestServer(t)
			id := createGroup(t, srv, &core.Group{DisplayName: "eng", Members: []core.Member{{Value: "u-1", Type: "User"}}}).ID

			response := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Groups/"+id,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBodyAs(t, protocol.PatchRequest{
					Schemas: []core.SchemaURI{protocol.SchemaPatchOp},
					Operations: []patch.Operation{
						{Op: patch.OpRemove, Path: `members[value eq "u-1"].type`},
					},
				}),
			))

			require.Equal(t, http.StatusBadRequest, response.StatusCode)
			assert.Equal(t, scimerrors.Mutability, ReadBodyAs[scimerrors.Error](t, response).ScimType)
		})

		// RFC 7643 Section 4.2: while values MAY be added or removed, sub-attributes of members are "immutable".
		for _, path := range []string{"members.value", `members[value eq "u-1"].value`} {
			t.Run("PATCH remove of the member value sub-attribute itself is a mutability error: "+path, func(t *testing.T) {
				srv := newTestServer(t)
				id := createGroup(t, srv, &core.Group{DisplayName: "eng", Members: []core.Member{{Value: "u-1", Type: "User"}}}).ID

				response := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Groups/"+id,
					WithBearerToken(validToken),
					WithContentType(protocol.MediaType),
					WithRequestBodyAs(t, protocol.PatchRequest{
						Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
						Operations: []patch.Operation{{Op: patch.OpRemove, Path: path}},
					}),
				))

				require.Equal(t, http.StatusBadRequest, response.StatusCode)
				assert.Equal(t, scimerrors.Mutability, ReadBodyAs[scimerrors.Error](t, response).ScimType)
			})
		}

		// RFC 7643 Section 7: an immutable attribute SHALL NOT be updated.
		for _, op := range []patch.Operation{
			{Op: patch.OpReplace, Path: `members[value eq "u-1"].value`, Value: json.RawMessage(`"u-2"`)},
			{Op: patch.OpReplace, Path: `members[value eq "u-1"].value`, Value: json.RawMessage(`null`)},
			{Op: patch.OpReplace, Path: `members[value eq "u-1"]`, Value: json.RawMessage(`{"value":"u-2"}`)},
		} {
			t.Run("PATCH replace that changes the member value sub-attribute is a mutability error: "+op.Path, func(t *testing.T) {
				srv := newTestServer(t)
				id := createGroup(t, srv, &core.Group{DisplayName: "eng", Members: []core.Member{{Value: "u-1", Type: "User"}}}).ID

				response := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Groups/"+id,
					WithBearerToken(validToken),
					WithContentType(protocol.MediaType),
					WithRequestBodyAs(t, protocol.PatchRequest{
						Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
						Operations: []patch.Operation{op},
					}),
				))

				require.Equal(t, http.StatusBadRequest, response.StatusCode)
				assert.Equal(t, scimerrors.Mutability, ReadBodyAs[scimerrors.Error](t, response).ScimType)
			})
		}

		t.Run("PATCH remove of an unassigned immutable sub-attribute of a matched member is a no-op", func(t *testing.T) {
			srv := newTestServer(t)
			id := createGroup(t, srv, &core.Group{DisplayName: "eng", Members: []core.Member{{Value: "u-1"}}}).ID

			response := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Groups/"+id,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBodyAs(t, protocol.PatchRequest{
					Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
					Operations: []patch.Operation{{Op: patch.OpRemove, Path: `members[value eq "u-1"].type`}},
				}),
			))

			require.Equal(t, http.StatusOK, response.StatusCode)
		})

		t.Run("resending a member by value and display alone twice keeps $ref and type", func(t *testing.T) {
			srv := newTestServer(t)
			id := createGroup(t, srv, &core.Group{DisplayName: "eng", Members: []core.Member{{Value: "u-1", Type: "User", Ref: "https://example.com/v2/Users/u-1"}}}).ID

			resend := func() *http.Response {
				return Response(t, srv, Request(t, srv, http.MethodPut, basePath+"/Groups/"+id,
					WithBearerToken(validToken),
					WithContentType(protocol.MediaType),
					WithRequestBody([]byte(`{"displayName":"eng","members":[{"value":"u-1","display":"alice@example.com"}]}`)),
				))
			}

			first := resend()
			require.Equal(t, http.StatusOK, first.StatusCode)

			second := resend()
			require.Equal(t, http.StatusOK, second.StatusCode)
			assert.Equal(t, []core.Member{{Value: "u-1", Type: "User", Ref: "https://example.com/v2/Users/u-1"}}, ReadBodyAs[core.Group](t, second).Members)
		})
	})

	// RFC 7644 Section 3.5.1: if an immutable value is already set, the input value(s) MUST match.
	t.Run("group member mutability is linear in member count", func(t *testing.T) {
		srv := newTestServer(t)
		grow := func(n int) time.Duration {
			original := make([]core.Member, n)
			for i := range original {
				original[i] = core.Member{Value: "dup", Type: "User"}
			}
			id := createGroup(t, srv, &core.Group{DisplayName: "eng", Members: original}).ID

			changed := make([]core.Member, n)
			for i := range changed {
				changed[i] = core.Member{Value: "dup", Type: "Group"}
			}
			changed[n-1] = core.Member{Value: "dup", Type: "User"}
			start := time.Now()
			response := Response(t, srv, Request(t, srv, http.MethodPut, basePath+"/Groups/"+id,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBodyAs(t, core.Group{DisplayName: "eng", Members: changed}),
			))
			elapsed := time.Since(start)

			require.Equal(t, http.StatusOK, response.StatusCode)
			return elapsed
		}

		small := min(grow(200), grow(200), grow(200))
		large := min(grow(800), grow(800), grow(800))

		assert.Less(t, float64(large)/float64(small), 8.0, "immutable member validation must not be quadratic in duplicate values")
	})

	// RFC 7644 Section 3.5.1: if an immutable value is already set, the input value(s) MUST match.
	t.Run("concurrent versionless PUTs cannot both set an immutable value", func(t *testing.T) {
		gate := newRaceGate(2)
		srv := newTestServer(t, withUpdateGate(gate))
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		put := func(employeeNumber string) *http.Request {
			return Request(t, srv, http.MethodPut, basePath+"/Users/"+id,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBodyAs(t, core.User{UserName: "bjensen", EnterpriseUser: &core.EnterpriseUser{EmployeeNumber: employeeNumber}}),
			)
		}

		responses := ConcurrentResponses(t, srv, put("a"), put("b"))
		statuses := statusCodes(responses)
		slices.Sort(statuses)
		require.Equal(t, []int{http.StatusOK, http.StatusConflict}, statuses)

		winner := "a"
		if responses[0].StatusCode != http.StatusOK {
			winner = "b"
		}
		final := ReadBodyAs[core.User](t, Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users/"+id, WithBearerToken(validToken))))
		assert.Equal(t, winner, final.EnterpriseUser.EmployeeNumber)
	})

	// RFC 7644 3.5.1 Replacing with PUT, 3.5.2 Modifying with PATCH
	t.Run("replace and patch get the resource once", func(t *testing.T) {
		newCountingUser := func(t *testing.T) (*httptest.Server, *countingRepository, string) {
			t.Helper()
			schemas := core.Schemas{core.NewSchema(core.SchemaUser).With(userAttributes()...)}
			repository := &countingRepository{Repository: server.NewRepository[*core.User](basePath+"/Users", schemas)}
			srv := Server(t, server.New(basePath, fullServiceProviderConfig(),
				server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, userAttributes()...).WithRepository(repository)),
			))
			id, _ := create(t, srv, &core.User{UserName: "bjensen"})
			return srv, repository, id
		}

		t.Run("PUT", func(t *testing.T) {
			srv, repository, id := newCountingUser(t)
			response := Response(t, srv, Request(t, srv, http.MethodPut, basePath+"/Users/"+id,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBody([]byte(`{"userName":"bjensen2"}`)),
			))
			require.Equal(t, http.StatusOK, response.StatusCode)
			assert.Equal(t, 1, repository.reads)
		})

		t.Run("PATCH", func(t *testing.T) {
			srv, repository, id := newCountingUser(t)
			patchUser(t, srv, id, patch.Operation{Op: patch.OpReplace, Path: "userType", Value: json.RawMessage(`"employee"`)})
			assert.Equal(t, 1, repository.reads)
		})
	})

	// RFC 7644 Section 3.5.1: if an immutable value is already set, the input value(s) MUST match.
	t.Run("immutable sub-attribute without a value sub-attribute", func(t *testing.T) {
		createGadget := func(t *testing.T, srv *httptest.Server) string {
			t.Helper()
			created := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Gadgets",
				WithBearerToken(validToken), WithContentType(protocol.MediaType),
				WithRequestBody([]byte(`{"parts":[{"serial":"s-1","code":"A"}]}`)),
			))
			require.Equal(t, http.StatusCreated, created.StatusCode)
			id, _ := ReadBodyAs[map[string]any](t, created)["id"].(string)
			return id
		}

		t.Run("rejects a changed code", func(t *testing.T) {
			t.Skip("MUST: immutability of a multi-valued sub-attribute is left to the repository, so the default server accepts the change")
		})

		t.Run("PUT omitting code keeps its stored value", func(t *testing.T) {
			srv := newTestServer(t)
			id := createGadget(t, srv)

			response := Response(t, srv, Request(t, srv, http.MethodPut, basePath+"/Gadgets/"+id,
				WithBearerToken(validToken), WithContentType(protocol.MediaType),
				WithRequestBody([]byte(`{"parts":[{"serial":"s-1"}]}`)),
			))

			require.Equal(t, http.StatusOK, response.StatusCode)
			assert.Equal(t, []part{{Serial: "s-1", Code: "A"}}, ReadBodyAs[gadget](t, response).Parts)
		})

		t.Run("PATCH replace omitting code keeps its stored value", func(t *testing.T) {
			srv := newTestServer(t)
			id := createGadget(t, srv)

			response := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Gadgets/"+id,
				WithBearerToken(validToken), WithContentType(protocol.MediaType),
				WithRequestBodyAs(t, protocol.PatchRequest{Schemas: []core.SchemaURI{protocol.SchemaPatchOp}, Operations: []patch.Operation{
					{Op: patch.OpReplace, Path: "parts", Value: json.RawMessage(`[{"serial":"s-1"}]`)},
				}}),
			))

			require.Equal(t, http.StatusOK, response.StatusCode)
			assert.Equal(t, []part{{Serial: "s-1", Code: "A"}}, ReadBodyAs[gadget](t, response).Parts)
		})
	})
}

// RFC 7644 3.5.2 Modifying with PATCH
func TestRFC7644ModifyingWithPATCH(t *testing.T) {
	// RFC 7644 Section 3.5.2: on successful completion, the server MUST return 200 OK and the entire resource, or MAY return 204.
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

	// RFC 7644 Section 3.5.2: a server MAY return 204 with no body for a successful PATCH.
	t.Run("returns 204 with an ETag for a group member add or remove, and is case-insensitive about the op and the path", func(t *testing.T) {
		srv := newTestServer(t)
		group := createGroup(t, srv, &core.Group{DisplayName: "eng", Members: []core.Member{{Value: "u-1", Type: "User"}}})

		for _, test := range []struct {
			name string
			op   patch.Operation
		}{
			{"add", patch.Operation{Op: patch.OpAdd, Path: "Members", Value: json.RawMessage(`[{"value":"u-2","type":"User"}]`)}},
			{"add with a capitalized op", patch.Operation{Op: patch.Op("Add"), Path: "members", Value: json.RawMessage(`[{"value":"u-3","type":"User"}]`)}},
			{"remove", patch.Operation{Op: patch.OpRemove, Path: `MEMBERS[value eq "u-2"]`}},
		} {
			t.Run(test.name, func(t *testing.T) {
				response := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Groups/"+group.ID,
					WithBearerToken(validToken),
					WithContentType(protocol.MediaType),
					WithRequestBodyAs(t, protocol.PatchRequest{
						Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
						Operations: []patch.Operation{test.op},
					}),
				))

				require.Equal(t, http.StatusNoContent, response.StatusCode)
				assert.NotEmpty(t, response.Header.Get("ETag"))
				body, err := io.ReadAll(response.Body)
				require.NoError(t, err)
				assert.Empty(t, body)
			})
		}
	})

	// RFC 7644 Section 3.5.2: the server MUST return 200 if the "attributes" parameter is specified.
	t.Run("returns 200 with a body for a group member patch when attributes or excludedAttributes is requested", func(t *testing.T) {
		srv := newTestServer(t)
		group := createGroup(t, srv, &core.Group{DisplayName: "eng", Members: []core.Member{{Value: "u-1", Type: "User"}}})

		for _, query := range []string{"?attributes=displayName", "?excludedAttributes=displayName"} {
			t.Run(query, func(t *testing.T) {
				response := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Groups/"+group.ID+query,
					WithBearerToken(validToken),
					WithContentType(protocol.MediaType),
					WithRequestBodyAs(t, protocol.PatchRequest{
						Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
						Operations: []patch.Operation{{Op: patch.OpAdd, Path: "members", Value: json.RawMessage(`[{"value":"u-2","type":"User"}]`)}},
					}),
				))

				require.Equal(t, http.StatusOK, response.StatusCode)
				body, err := io.ReadAll(response.Body)
				require.NoError(t, err)
				assert.NotEmpty(t, body)
			})
		}
	})

	// RFC 7644 Section 3.5.2: 204 is scoped to a patch where every operation touches only "members".
	t.Run("returns 200 with a body when a group member patch is mixed with a non-member operation", func(t *testing.T) {
		srv := newTestServer(t)
		group := createGroup(t, srv, &core.Group{DisplayName: "eng", Members: []core.Member{{Value: "u-1", Type: "User"}}})

		response := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Groups/"+group.ID,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas: []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{
					{Op: patch.OpAdd, Path: "members", Value: json.RawMessage(`[{"value":"u-2","type":"User"}]`)},
					{Op: patch.OpReplace, Path: "displayName", Value: json.RawMessage(`"renamed"`)},
				},
			}),
		))

		require.Equal(t, http.StatusOK, response.StatusCode)
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		assert.NotEmpty(t, body)
	})

	// RFC 7644 Section 3.5.2: a path-less replace is a different operation shape and does not qualify for 204.
	t.Run("returns 200 with a body for a path-less replace that only sets members", func(t *testing.T) {
		srv := newTestServer(t)
		group := createGroup(t, srv, &core.Group{DisplayName: "eng", Members: []core.Member{{Value: "u-1", Type: "User"}}})

		response := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Groups/"+group.ID,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{{Op: patch.OpReplace, Value: json.RawMessage(`{"members":[{"value":"u-2","type":"User"}]}`)}},
			}),
		))

		require.Equal(t, http.StatusOK, response.StatusCode)
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		assert.NotEmpty(t, body)
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

	// RFC 7644 Section 3.5.2: a client MUST NOT modify a readOnly attribute.
	t.Run("rejects a readOnly sub-attribute even when the value filter matches nothing", func(t *testing.T) {
		srv := newTestServer(t)
		id := createWidget(t, srv, &widget{Name: "gizmo", Parts: []part{{Serial: "s-1"}}})["id"].(string)

		request := Request(t, srv, http.MethodPatch, basePath+"/Widgets/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{{Op: patch.OpReplace, Path: `parts[serial eq "none"].inspector`, Value: json.RawMessage(`"x"`)}},
			}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.Mutability, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7643 Section 7: a readOnly attribute SHALL NOT be modified.
	t.Run("keeps readOnly sub-attributes the operations do not target", func(t *testing.T) {
		srv := newTestServer(t)
		id := createWidget(t, srv, &widget{Name: "gizmo", Parts: []part{{Serial: "s-1"}}})["id"].(string)

		request := Request(t, srv, http.MethodPatch, basePath+"/Widgets/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{{Op: patch.OpReplace, Path: "name", Value: json.RawMessage(`"doohickey"`)}},
			}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, []part{{Serial: "s-1", Inspector: "qa-bot"}}, ReadBodyAs[widget](t, response).Parts)
	})

	// RFC 7644 Section 3.5.2: a client MUST NOT modify a readOnly attribute.
	t.Run("rejects a readOnly sub-attribute inside the value", func(t *testing.T) {
		srv := newTestServer(t)
		id := createWidget(t, srv, &widget{Name: "gizmo", Parts: []part{{Serial: "s-1"}}})["id"].(string)

		for _, operation := range []patch.Operation{
			{Op: patch.OpReplace, Path: "parts", Value: json.RawMessage(`[{"serial":"s-1","inspector":"mallory"}]`)},
			{Op: patch.OpAdd, Path: "parts", Value: json.RawMessage(`{"serial":"s-2","inspector":"mallory"}`)},
			{Op: patch.OpAdd, Value: json.RawMessage(`{"parts":[{"serial":"s-2","inspector":"mallory"}]}`)},
		} {
			request := Request(t, srv, http.MethodPatch, basePath+"/Widgets/"+id,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBodyAs(t, protocol.PatchRequest{
					Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
					Operations: []patch.Operation{operation},
				}),
			)
			response := Response(t, srv, request)

			require.Equal(t, http.StatusBadRequest, response.StatusCode, "operation: %s", operation.Value)
			assert.Equal(t, scimerrors.Mutability, ReadBodyAs[scimerrors.Error](t, response).ScimType, "operation: %s", operation.Value)
		}
	})

	// RFC 7643 Section 4.1.1: a password is used to "compare (i.e., filter for equality)".
	t.Run("rejects any operator but eq on a writeOnly attribute in a value filter", func(t *testing.T) {
		srv := newTestServer(t)
		id := createWidget(t, srv, &widget{Name: "gizmo", Parts: []part{{Serial: "s-1", Code: "c0de"}}, Keys: []map[string]any{{"value": "k3y"}}})["id"].(string)

		for _, path := range []string{`parts[code sw "c"].serial`, `parts[code pr].serial`, `keys[value sw "k"].value`, `keys[value pr]`} {
			request := Request(t, srv, http.MethodPatch, basePath+"/Widgets/"+id,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBodyAs(t, protocol.PatchRequest{
					Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
					Operations: []patch.Operation{{Op: patch.OpReplace, Path: path, Value: json.RawMessage(`"x"`)}},
				}),
			)
			response := Response(t, srv, request)

			require.Equal(t, http.StatusBadRequest, response.StatusCode, path)
			assert.Equal(t, scimerrors.InvalidFilter, ReadBodyAs[scimerrors.Error](t, response).ScimType, path)
		}
	})

	// RFC 7644 Section 3.12: invalidFilter when the attribute and filter comparison combination is not supported.
	t.Run("rejects a value path filter on a single-valued complex attribute", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "alice", Name: core.Name{GivenName: "alice"}})

		for _, op := range []patch.Operation{
			{Op: patch.OpReplace, Path: `name[givenName eq "alice"].familyName`, Value: json.RawMessage(`"x"`)},
			{Op: patch.OpAdd, Path: `name[givenName eq "alice"].familyName`, Value: json.RawMessage(`"x"`)},
			{Op: patch.OpRemove, Path: `name[givenName eq "alice"]`},
		} {
			response := Response(t, srv, patchRequest(t, srv, "Users/"+id, op))

			require.Equal(t, http.StatusBadRequest, response.StatusCode, op.Op)
			assert.Equal(t, scimerrors.InvalidFilter, ReadBodyAs[scimerrors.Error](t, response).ScimType, op.Op)
		}
	})

	// RFC 7644 Section 3.4.2.2: a simple multi-valued attribute is filtered by "value" alone.
	t.Run("rejects a value filter that is not on a sub-attribute or the value", func(t *testing.T) {
		srv := newTestServer(t)
		id := createWidget(t, srv, &widget{Name: "gizmo", Tags: []any{"red"}, Parts: []part{{Serial: "s-1"}}})["id"].(string)

		for _, path := range []string{`tags[type eq "x"]`, `tags[value.x eq "a"]`, `tags[urn:test:widget:value eq "red"]`, `parts[urn:test:widget:serial eq "s-1"]`} {
			response := Response(t, srv, patchRequest(t, srv, "Widgets/"+id, patch.Operation{Op: patch.OpRemove, Path: path}))

			require.Equal(t, http.StatusBadRequest, response.StatusCode, path)
			assert.Equal(t, scimerrors.InvalidFilter, ReadBodyAs[scimerrors.Error](t, response).ScimType, path)
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

	// RFC 7644 Section 3.12: invalidSyntax when the request body message structure was invalid.
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

	// RFC 7644 Section 3.12: invalidSyntax when the request body message structure was invalid.
	t.Run("rejects a malformed JSON body before looking up the resource", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodPatch, basePath+"/Users/unknown",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBody([]byte(`{not-json`)),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidSyntax, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7644 Section 3.12: invalidPath when the "path" attribute was invalid or malformed.
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

	// RFC 7644 Section 3.5.2: PATH = attrPath / valuePath [subAttr]
	t.Run("rejects a patch path with a sub-attribute on both sides of a value filter", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		request := Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{{Op: patch.OpReplace, Path: `emails.value[type eq "work"].display`, Value: json.RawMessage(`"x"`)}},
			}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidPath, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7644 Section 3.4.2.2: the bracketed filter is "based upon sub-attributes of the parent attribute".
	t.Run("rejects a patch path with a value filter on a sub-attribute", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen", Emails: []core.Email{{Value: "bjensen@example.com", Type: "work"}}})

		request := Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{{Op: patch.OpReplace, Path: `emails.value[type eq "work"]`, Value: json.RawMessage(`"x@example.com"`)}},
			}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidPath, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7644 Section 3.12: mutability when the modification is not compatible with the target attribute's mutability.
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

	// RFC 7644 Section 3.12: uniqueness when one or more of the attribute values are already in use.
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

	// RFC 7644 Section 3.12: 404 when the specified resource does not exist.
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

	// RFC 7644 Section 3.12: invalidSyntax when the request body message structure was invalid.
	t.Run("a malformed body on an unknown id is a 400, not a 404", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodPatch, basePath+"/Groups/does-not-exist",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBody([]byte(`{not-json`)),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidSyntax, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7644 Section 3.12: 412 when the update failed because the resource has changed on the server.
	t.Run("rejects a patch with a stale or empty If-Match", func(t *testing.T) {
		for _, ifMatch := range []string{`W/"stale"`, ","} {
			srv := newTestServer(t)
			id, _ := create(t, srv, &core.User{UserName: "bjensen"})

			request := Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithHeader("If-Match", ifMatch),
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

			assert.Equal(t, http.StatusPreconditionFailed, response.StatusCode, ifMatch)
		}
	})

	// RFC 7643 Section 4.2: while values MAY be added or removed, sub-attributes of members are "immutable".
	t.Run("rejects changing an immutable value but allows adding and removing members", func(t *testing.T) {
		extension := string(core.SchemaEnterpriseUser)
		active := true
		user := &core.User{UserName: "bjensen", EnterpriseUser: &core.EnterpriseUser{EmployeeNumber: "E1"}}
		gear := &kit{Active: &active, Name: &core.Name{GivenName: "gear"}, Parts: []part{{Serial: "s-1"}, {Serial: "s-2"}}}
		team := &core.Group{DisplayName: "eng", Members: []core.Member{{Value: "u-1", Type: "User"}, {Value: "u-2", Type: "User"}}}

		for _, test := range []struct {
			name     string
			endpoint string
			resource any
			op       patch.Operation
			status   int
		}{
			{"rejects a changed extension value", "/Users", user, patch.Operation{Op: patch.OpReplace, Path: extension + ":employeeNumber", Value: json.RawMessage(`"E2"`)}, http.StatusBadRequest},
			{"accepts the same extension value", "/Users", user, patch.Operation{Op: patch.OpReplace, Path: extension + ":employeeNumber", Value: json.RawMessage(`"E1"`)}, http.StatusOK},
			{"accepts adding a member", "/Kits", gear, patch.Operation{Op: patch.OpAdd, Path: "parts", Value: json.RawMessage(`[{"serial":"s-3"}]`)}, http.StatusOK},
			{"accepts removing a member", "/Kits", gear, patch.Operation{Op: patch.OpRemove, Path: `parts[serial eq "s-1"]`}, http.StatusOK},
			{"accepts adding a group member", "/Groups", team, patch.Operation{Op: patch.OpAdd, Path: "members", Value: json.RawMessage(`[{"value":"u-3","type":"User"}]`)}, http.StatusNoContent},
			{"accepts adding a group member that repeats a value with another type", "/Groups", team, patch.Operation{Op: patch.OpAdd, Path: "members", Value: json.RawMessage(`[{"value":"u-1","type":"Group"}]`)}, http.StatusNoContent},
			{"accepts a group member type that differs only in case", "/Groups", team, patch.Operation{Op: patch.OpReplace, Path: `members[value eq "u-1"].type`, Value: json.RawMessage(`"user"`)}, http.StatusOK},
			{"accepts an extension value that differs only in case", "/Users", user, patch.Operation{Op: patch.OpReplace, Path: extension + ":employeeNumber", Value: json.RawMessage(`"e1"`)}, http.StatusOK},
			{"accepts removing a group member", "/Groups", team, patch.Operation{Op: patch.OpRemove, Path: `members[value eq "u-1"]`}, http.StatusNoContent},
			{"accepts replacing the group members", "/Groups", team, patch.Operation{Op: patch.OpReplace, Path: "members", Value: json.RawMessage(`[{"value":"u-3","type":"User"}]`)}, http.StatusOK},
			{"rejects a changed immutable sub-attribute of a group member", "/Groups", team, patch.Operation{Op: patch.OpReplace, Path: `members[value eq "u-1"].type`, Value: json.RawMessage(`"Group"`)}, http.StatusBadRequest},
		} {
			t.Run(test.name, func(t *testing.T) {
				srv := newTestServer(t)
				created := Response(t, srv, Request(t, srv, http.MethodPost, basePath+test.endpoint, WithBearerToken(validToken), WithContentType(protocol.MediaType), WithRequestBodyAs(t, test.resource)))
				require.Equal(t, http.StatusCreated, created.StatusCode)
				id, _ := ReadBodyAs[map[string]any](t, created)["id"].(string)

				request := Request(t, srv, http.MethodPatch, basePath+test.endpoint+"/"+id,
					WithBearerToken(validToken),
					WithContentType(protocol.MediaType),
					WithRequestBodyAs(t, protocol.PatchRequest{Schemas: []core.SchemaURI{protocol.SchemaPatchOp}, Operations: []patch.Operation{test.op}}),
				)

				response := Response(t, srv, request)
				require.Equal(t, test.status, response.StatusCode)
				if test.status == http.StatusBadRequest {
					assert.Equal(t, scimerrors.Mutability, ReadBodyAs[scimerrors.Error](t, response).ScimType)
				}
			})
		}
	})

	// RFC 7644 Section 3.5.2.1: if the user was already a member of this group, no changes should be made to the resource.
	t.Run("adding a group member that repeats a value with another type is a no-op", func(t *testing.T) {
		srv := newTestServer(t)
		id := createGroup(t, srv, &core.Group{DisplayName: "eng", Members: []core.Member{{Value: "u-1", Type: "User"}}}).ID

		response := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Groups/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{Schemas: []core.SchemaURI{protocol.SchemaPatchOp}, Operations: []patch.Operation{
				{Op: patch.OpAdd, Path: "members", Value: json.RawMessage(`[{"value":"u-1","type":"Group"}]`)},
			}}),
		))
		require.Equal(t, http.StatusNoContent, response.StatusCode)

		fetched := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Groups/"+id, WithBearerToken(validToken)))
		patched := ReadBodyAs[core.Group](t, fetched)
		require.Len(t, patched.Members, 1)
		assert.Equal(t, core.ResourceTypeName("User"), patched.Members[0].Type)
	})

	// RFC 7644 Section 3.5.2: setting a value's "primary" to "true" SHALL cause the server to set "primary" to "false" for any other values.
	t.Run("sets primary to false on the other values when a value becomes primary", func(t *testing.T) {
		srv := newTestServer(t)
		primary := true
		id, _ := create(t, srv, &core.User{UserName: "bjensen", Emails: []core.Email{{Value: "a@example.com", Type: "work", Primary: &primary}}})

		patched := patchUser(t, srv, id, patch.Operation{Op: patch.OpAdd, Path: "emails", Value: json.RawMessage(`[{"value":"b@example.com","type":"home","primary":true}]`)})

		require.Len(t, patched.Emails, 2)
		assert.False(t, *patched.Emails[0].Primary)
		assert.True(t, *patched.Emails[1].Primary)
	})

	// RFC 7644 Section 3.5.2: setting a value's "primary" to "true" SHALL cause the server to set "primary" to "false" for any other values.
	t.Run("makes a stored value primary through a value path", func(t *testing.T) {
		primaries := func(emails []core.Email) []string {
			values := []string{}
			for _, email := range emails {
				if email.Primary != nil && *email.Primary {
					values = append(values, email.Value)
				}
			}
			return values
		}
		stored := patch.Operation{Op: patch.OpReplace, Path: `emails[value eq "c@example.com"].primary`, Value: json.RawMessage(`true`)}
		for _, test := range []struct {
			name       string
			operations []patch.Operation
			want       string
		}{
			{"alone", []patch.Operation{stored}, "c@example.com"},
			{"then a new primary", []patch.Operation{stored, {Op: patch.OpAdd, Path: "emails", Value: json.RawMessage(`[{"value":"e@example.com","type":"other","primary":true}]`)}}, "e@example.com"},
		} {
			srv := newTestServer(t)
			primary := true
			id, _ := create(t, srv, &core.User{UserName: "bjensen", Emails: []core.Email{
				{Value: "a@example.com", Type: "work", Primary: &primary},
				{Value: "c@example.com", Type: "home"},
			}})

			patched := patchUser(t, srv, id, test.operations...)

			assert.Equal(t, []string{test.want}, primaries(patched.Emails), test.name)
		}
	})

	// RFC 7644 Section 3.5.2: the body of each request MAY contain multiple operations and SHALL be treated as atomic.
	t.Run("leaves the resource unchanged when a later operation fails", func(t *testing.T) {
		srv := newTestServer(t)
		primary := true
		id, _ := create(t, srv, &core.User{UserName: "bjensen", DisplayName: "Babs", Emails: []core.Email{{Value: "a@example.com", Primary: &primary}}})

		response := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{Schemas: []core.SchemaURI{protocol.SchemaPatchOp}, Operations: []patch.Operation{
				{Op: patch.OpReplace, Path: "displayName", Value: json.RawMessage(`"Changed"`)},
				{Op: patch.OpAdd, Path: "emails", Value: json.RawMessage(`[{"value":"b@example.com","primary":true}]`)},
				{Op: patch.OpReplace, Path: "groups", Value: json.RawMessage(`[{"value":"g-1"}]`)},
			}}),
		))
		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.Mutability, ReadBodyAs[scimerrors.Error](t, response).ScimType)

		fetched := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users/"+id, WithBearerToken(validToken)))
		require.Equal(t, http.StatusOK, fetched.StatusCode)
		user := ReadBodyAs[core.User](t, fetched)
		assert.Equal(t, "Babs", user.DisplayName)
		assert.Equal(t, []core.Email{{Value: "a@example.com", Primary: &primary}}, user.Emails)
	})

	// RFC 7643 Section 2.1: attribute names are case insensitive and the character set is US-ASCII.
	t.Run("rejects a value with an attribute name that is not US-ASCII or repeats in another case", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})
		extension := string(core.SchemaEnterpriseUser)

		for _, operation := range []patch.Operation{
			{Op: patch.OpReplace, Value: json.RawMessage(`{"displayName":"a","DISPLAYNAME":"b"}`)},
			{Op: patch.OpReplace, Path: "name", Value: json.RawMessage(`{"givenName":"a","GIVENNAME":"b"}`)},
			{Op: patch.OpAdd, Path: "emails", Value: json.RawMessage(`[{"value":"a@example.com","VALUE":"b@example.com"}]`)},
			{Op: patch.OpAdd, Value: json.RawMessage(`{"` + extension + `":{"department":"a","DEPARTMENT":"b"}}`)},
			{Op: patch.OpAdd, Value: json.RawMessage(`{"nick\u00e9":"x"}`)},
		} {
			response := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBodyAs(t, protocol.PatchRequest{Schemas: []core.SchemaURI{protocol.SchemaPatchOp}, Operations: []patch.Operation{operation}}),
			))

			require.Equal(t, http.StatusBadRequest, response.StatusCode, string(operation.Value))
			assert.Equal(t, scimerrors.InvalidSyntax, ReadBodyAs[scimerrors.Error](t, response).ScimType, string(operation.Value))
		}
	})

	// RFC 8259 Section 4: the names within an object SHOULD be unique.
	t.Run("rejects a value with a duplicate attribute name", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		response := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{Schemas: []core.SchemaURI{protocol.SchemaPatchOp}, Operations: []patch.Operation{
				{Op: patch.OpReplace, Value: json.RawMessage(`{"displayName":"a","displayName":"b"}`)},
			}}),
		))

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidSyntax, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7643 Section 4.2: while values MAY be added or removed, sub-attributes of members are "immutable".
	t.Run("group member patch", func(t *testing.T) {
		newGroupServer := func(t *testing.T) *httptest.Server {
			t.Helper()
			return Server(t, newGroupHandler(t, server.NewRepository[*core.Group](basePath+"/Groups", groupSchemas())))
		}
		eng := &core.Group{DisplayName: "eng", Members: []core.Member{{Value: "u-1", Type: "User"}}}
		patchMembers := func(t *testing.T, srv *httptest.Server, id string, ops []patch.Operation, options ...Option[*http.Request]) *http.Response {
			t.Helper()
			options = append([]Option[*http.Request]{
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBodyAs(t, protocol.PatchRequest{Schemas: []core.SchemaURI{protocol.SchemaPatchOp}, Operations: ops}),
			}, options...)
			return Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Groups/"+id, options...))
		}

		for _, scenario := range []struct {
			name            string
			ops             []patch.Operation
			expectedMembers []core.Member
		}{
			{
				"add",
				[]patch.Operation{{Op: patch.OpAdd, Path: "members", Value: json.RawMessage(`[{"value":"u-2","type":"User"}]`)}},
				[]core.Member{{Value: "u-1", Type: "User"}, {Value: "u-2", Type: "User"}},
			},
			{
				"add with a capitalized key",
				[]patch.Operation{{Op: patch.OpAdd, Path: "members", Value: json.RawMessage(`[{"Value":"u-2","Type":"User"}]`)}},
				[]core.Member{{Value: "u-1", Type: "User"}, {Value: "u-2", Type: "User"}},
			},
			{
				"add with an unknown sub-attribute",
				[]patch.Operation{{Op: patch.OpAdd, Path: "members", Value: json.RawMessage(`[{"value":"u-2","type":"User","display":"x"}]`)}},
				[]core.Member{{Value: "u-1", Type: "User"}, {Value: "u-2", Type: "User"}},
			},
			{
				"add of an already-present value is a no-op",
				[]patch.Operation{{Op: patch.OpAdd, Path: "members", Value: json.RawMessage(`[{"value":"u-1","type":"User"}]`)}},
				[]core.Member{{Value: "u-1", Type: "User"}},
			},
			{
				"remove of a value that is not present",
				[]patch.Operation{{Op: patch.OpRemove, Path: `members[value eq "does-not-exist"]`}},
				[]core.Member{{Value: "u-1", Type: "User"}},
			},
		} {
			t.Run(scenario.name, func(t *testing.T) {
				srv := newGroupServer(t)
				group := createGroup(t, srv, eng)

				response := patchMembers(t, srv, group.ID, scenario.ops)
				require.Equal(t, http.StatusNoContent, response.StatusCode)
				assert.NotEmpty(t, response.Header.Get("ETag"))

				fetched := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Groups/"+group.ID, WithBearerToken(validToken)))
				patched := ReadBodyAs[core.Group](t, fetched)
				assert.ElementsMatch(t, scenario.expectedMembers, patched.Members)
			})
		}

		t.Run("a valid member add on an unknown id is a 404", func(t *testing.T) {
			srv := newGroupServer(t)

			response := patchMembers(t, srv, "does-not-exist", []patch.Operation{{Op: patch.OpAdd, Path: "members", Value: json.RawMessage(`[{"value":"u-2","type":"User"}]`)}})

			require.Equal(t, http.StatusNotFound, response.StatusCode)
		})

		// RFC 7643 Section 4.2: an immutable member cannot be removed and re-added with a different type.
		t.Run("rejects removing and re-adding the same identity with a different type", func(t *testing.T) {
			t.Skip("MUST: member sub-attribute immutability is left to the Group repository, so the re-added type is accepted")
		})

		// RFC 7643 Section 2.2: an added member's "type" must be a canonical value.
		t.Run("rejects an added member whose type is not canonical", func(t *testing.T) {
			srv := newGroupServer(t)
			group := createGroup(t, srv, eng)

			response := patchMembers(t, srv, group.ID, []patch.Operation{{Op: patch.OpAdd, Path: "members", Value: json.RawMessage(`[{"value":"u-2","type":"Bogus"}]`)}})

			require.Equal(t, http.StatusBadRequest, response.StatusCode)
			assert.Equal(t, scimerrors.InvalidValue, ReadBodyAs[scimerrors.Error](t, response).ScimType)
		})

		// RFC 7643 Section 2.3: an added member whose "value" is not a string is a 4xx, not a raw decode error.
		t.Run("rejects an added member whose value is the wrong type", func(t *testing.T) {
			srv := newGroupServer(t)
			group := createGroup(t, srv, eng)

			response := patchMembers(t, srv, group.ID, []patch.Operation{{Op: patch.OpAdd, Path: "members", Value: json.RawMessage(`[{"value":123}]`)}})

			require.Equal(t, http.StatusBadRequest, response.StatusCode)
		})

		t.Run("rejects a member add with a stale If-Match", func(t *testing.T) {
			srv := newGroupServer(t)
			group := createGroup(t, srv, eng)

			response := patchMembers(t, srv, group.ID, []patch.Operation{{Op: patch.OpAdd, Path: "members", Value: json.RawMessage(`[{"value":"u-2","type":"User"}]`)}}, WithHeader("If-Match", `W/"stale"`))

			require.Equal(t, http.StatusPreconditionFailed, response.StatusCode)
		})
	})

	// RFC 7644 Section 3.5.2: a PATCH path may be qualified with the schema extension URN.
	t.Run("patches an extension attribute by its URN-qualified path", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "patched", EnterpriseUser: &core.EnterpriseUser{Department: "eng"}})

		patched := patchUser(t, srv, id, patch.Operation{
			Op:    patch.OpReplace,
			Path:  string(core.SchemaEnterpriseUser) + ":department",
			Value: json.RawMessage(`"sales"`),
		})

		require.NotNil(t, patched.EnterpriseUser)
		assert.Equal(t, "sales", patched.EnterpriseUser.Department)
	})

	// RFC 7644 Section 3.5.2: with no path, the value holds attributes keyed by the schema extension URN.
	t.Run("patches an extension object when the path is omitted", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "merged", EnterpriseUser: &core.EnterpriseUser{EmployeeNumber: "9"}})

		patched := patchUser(t, srv, id, patch.Operation{
			Op:    patch.OpReplace,
			Value: json.RawMessage(`{"` + string(core.SchemaEnterpriseUser) + `":{"department":"ops"}}`),
		})

		require.NotNil(t, patched.EnterpriseUser)
		assert.Equal(t, "ops", patched.EnterpriseUser.Department)
		assert.Equal(t, "9", patched.EnterpriseUser.EmployeeNumber)
	})
}

// RFC 7644 3.5.2.1 Add Operation
func TestRFC7644AddOperation(t *testing.T) {
	// RFC 7644 Section 3.5.2.1: the operation MUST contain a "value" member whose content specifies the value to be added.
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

	// RFC 7644 Section 3.5.2.1: if "path" is omitted, the target location is assumed to be the resource itself.
	t.Run("adds the attributes of the value when the path is omitted", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		patched := patchUser(t, srv, id, patch.Operation{Op: patch.OpAdd, Value: json.RawMessage(`{"userType":"employee"}`)})

		assert.Equal(t, "employee", patched.UserType)
	})

	// RFC 7643 Section 3: "schemas" lists the base schema and each extension present, so the server derives it.
	t.Run("ignores a schemas key in the value when the path is omitted", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen", EnterpriseUser: &core.EnterpriseUser{EmployeeNumber: "1"}})

		patched := patchUser(t, srv, id, patch.Operation{Op: patch.OpReplace, Value: json.RawMessage(`{"schemas":["` + string(core.SchemaUser) + `"],"userType":"employee"}`)})

		assert.Equal(t, "employee", patched.UserType)
		assert.Equal(t, []core.SchemaURI{core.SchemaUser, core.SchemaEnterpriseUser}, patched.Schemas)
	})

	// RFC 7644 Section 3.5.2.1: if the target location already contains the value specified, no changes SHOULD be made, and the modify timestamp SHALL NOT change.
	t.Run("does not append an email the target location already contains, and leaves meta.lastModified unchanged", func(t *testing.T) {
		srv := newTestServer(t)
		primary := true
		created := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Users",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, &core.User{UserName: "bjensen", Emails: []core.Email{{Value: "a@example.com", Type: "work", Primary: &primary}}}),
		))
		require.Equal(t, http.StatusCreated, created.StatusCode)
		user := ReadBodyAs[core.User](t, created)

		patched := patchUser(t, srv, user.ID, patch.Operation{Op: patch.OpAdd, Path: "emails", Value: json.RawMessage(`[{"value":"a@example.com","type":"work","primary":true}]`)})

		require.Len(t, patched.Emails, 1)
		assert.True(t, *patched.Emails[0].Primary)
		assert.Equal(t, user.Meta.Version, patched.Meta.Version)
		assert.Equal(t, user.Meta.LastModified, patched.Meta.LastModified)
	})

	// RFC 7644 Section 3.5.2.1: if the target location already contains the value specified, no changes SHOULD be made, and the modify timestamp SHALL NOT change.
	t.Run("does not append a group member the target location already contains, and leaves meta.lastModified unchanged", func(t *testing.T) {
		srv := newTestServer(t)
		group := createGroup(t, srv, &core.Group{DisplayName: "eng", Members: []core.Member{{Value: "u-1", Type: "User"}}})

		request := Request(t, srv, http.MethodPatch, basePath+"/Groups/"+group.ID,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{{Op: patch.OpAdd, Path: "members", Value: json.RawMessage(`[{"value":"u-1","type":"User"}]`)}},
			}),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusNoContent, response.StatusCode)
		fetched := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Groups/"+group.ID, WithBearerToken(validToken)))
		patched := ReadBodyAs[core.Group](t, fetched)
		assert.Len(t, patched.Members, 1)
		assert.Equal(t, group.Meta.Version, patched.Meta.Version)
		assert.Equal(t, group.Meta.LastModified, patched.Meta.LastModified)
	})

	// RFC 7644 Section 3.5.2.1: if the target location already contains the value specified, the modify timestamp SHALL NOT change.
	for _, tc := range []struct {
		name string
		op   patch.Op
		body string
	}{
		{"adding a member in another case", patch.OpAdd, `[{"Value":"u-1","Type":"User"}]`},
		{"adding a member with a sub-attribute the resource does not store", patch.OpAdd, `[{"value":"u-1","type":"User","display":"alice"}]`},
		{"replacing the members with the same member in another case", patch.OpReplace, `[{"Value":"u-1","Type":"User"}]`},
		{"replacing the members with the same member and a sub-attribute the resource does not store", patch.OpReplace, `[{"value":"u-1","type":"User","display":"alice"}]`},
	} {
		t.Run("leaves meta.version unchanged when "+tc.name, func(t *testing.T) {
			srv := newTestServer(t)
			group := createGroup(t, srv, &core.Group{DisplayName: "eng", Members: []core.Member{{Value: "u-1", Type: "User"}}})

			Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Groups/"+group.ID,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBodyAs(t, protocol.PatchRequest{
					Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
					Operations: []patch.Operation{{Op: tc.op, Path: "members", Value: json.RawMessage(tc.body)}},
				}),
			))

			fetched := ReadBodyAs[core.Group](t, Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Groups/"+group.ID, WithBearerToken(validToken))))
			assert.Equal(t, group.Meta.Version, fetched.Meta.Version)
		})
	}

	// RFC 7644 Section 3.5.2.1: a no-op patch SHALL NOT change the modify timestamp; RFC 7643 Section 3: "schemas" must keep reflecting the extension's data across that no-op.
	t.Run("a no-op patch on a resource with extension data leaves meta.lastModified and schemas unchanged", func(t *testing.T) {
		srv := newTestServer(t)
		primary := true
		created := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Users",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, &core.User{
				UserName:       "bjensen",
				Emails:         []core.Email{{Value: "a@example.com", Type: "work", Primary: &primary}},
				EnterpriseUser: &core.EnterpriseUser{EmployeeNumber: "E1"},
			}),
		))
		require.Equal(t, http.StatusCreated, created.StatusCode)
		user := ReadBodyAs[core.User](t, created)

		patched := patchUser(t, srv, user.ID, patch.Operation{Op: patch.OpAdd, Path: "emails", Value: json.RawMessage(`[{"value":"a@example.com","type":"work","primary":true}]`)})

		assert.Equal(t, user.Meta.Version, patched.Meta.Version)
		assert.Equal(t, user.Meta.LastModified, patched.Meta.LastModified)
		assert.Equal(t, []core.SchemaURI{core.SchemaUser, core.SchemaEnterpriseUser}, patched.Schemas)
	})
}

// RFC 7644 3.5.2.2 Remove Operation
func TestRFC7644RemoveOperation(t *testing.T) {
	// RFC 7644 Section 3.5.2.2: if the user was not a member of this group, no changes should be made and a success response should be returned.
	t.Run("removing a sub-attribute behind a filter that matches nothing changes nothing", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen", Emails: []core.Email{{Value: "a@example.com", Type: "work"}}})

		patched := patchUser(t, srv, id, patch.Operation{Op: patch.OpRemove, Path: `emails[type eq "home"].value`})

		assert.Equal(t, []core.Email{{Value: "a@example.com", Type: "work"}}, patched.Emails)
	})

	// RFC 7644 Section 3.5.2.2: the attribute at the target location and its associated value is removed.
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

	// RFC 7644 Section 3.5.2.2: "remove" selects its target by "path" alone and defines no "value".
	t.Run("rejects a remove that carries a value", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen", Emails: []core.Email{{Value: "a@example.com"}, {Value: "b@example.com"}}})

		response := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{{Op: patch.OpRemove, Path: "emails", Value: json.RawMessage(`[{"value":"a@example.com"}]`)}},
			}),
		))

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidSyntax, ReadBodyAs[scimerrors.Error](t, response).ScimType)
		stored := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users/"+id, WithBearerToken(validToken)))
		assert.Len(t, ReadBodyAs[core.User](t, stored).Emails, 2)
	})

	// RFC 7644 Section 3.5.2.2: if a read-only attribute is removed or becomes unassigned, the server SHALL return "mutability".
	t.Run("rejects a remove that unassigns a readOnly sub-attribute", func(t *testing.T) {
		srv := newTestServer(t)
		id := createWidget(t, srv, &widget{Name: "gizmo", Parts: []part{{Serial: "s-1"}}})["id"].(string)

		for _, path := range []string{"parts", `parts[serial eq "s-1"]`} {
			request := Request(t, srv, http.MethodPatch, basePath+"/Widgets/"+id,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBodyAs(t, protocol.PatchRequest{
					Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
					Operations: []patch.Operation{{Op: patch.OpRemove, Path: path}},
				}),
			)
			response := Response(t, srv, request)

			require.Equal(t, http.StatusBadRequest, response.StatusCode, "path: %s", path)
			assert.Equal(t, scimerrors.Mutability, ReadBodyAs[scimerrors.Error](t, response).ScimType, "path: %s", path)
		}
	})

	// RFC 7644 Section 3.5.2.2: if a required attribute is removed or becomes unassigned, the server SHALL return "mutability".
	t.Run("rejects a remove that unassigns a required attribute", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		response := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{{Op: patch.OpRemove, Path: "userName"}},
			}),
		))

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.Mutability, ReadBodyAs[scimerrors.Error](t, response).ScimType)
		stored := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users/"+id, WithBearerToken(validToken)))
		assert.Equal(t, "bjensen", ReadBodyAs[core.User](t, stored).UserName)
	})

	// RFC 7644 Section 3.5.2.2: a filtered remove that empties a required multi-valued attribute SHALL return "mutability".
	t.Run("rejects a filtered remove that empties a required multi-valued attribute", func(t *testing.T) {
		srv := newTestServer(t)
		active := true
		created := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Kits",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, &kit{Active: &active, Name: &core.Name{GivenName: "gear"}, Parts: []part{{Serial: "s-1"}}}),
		))
		require.Equal(t, http.StatusCreated, created.StatusCode)
		id, _ := ReadBodyAs[map[string]any](t, created)["id"].(string)

		response := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Kits/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{{Op: patch.OpRemove, Path: `parts[serial eq "s-1"]`}},
			}),
		))

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.Mutability, ReadBodyAs[scimerrors.Error](t, response).ScimType)
		stored := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Kits/"+id, WithBearerToken(validToken)))
		assert.Len(t, ReadBodyAs[kit](t, stored).Parts, 1)
	})

	// RFC 7644 Section 3.5.2.2: if "path" is unspecified, the operation fails with 400 and scimType "noTarget".
	t.Run("rejects a remove without a path with noTarget", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		response := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{Schemas: []core.SchemaURI{protocol.SchemaPatchOp}, Operations: []patch.Operation{{Op: patch.OpRemove}}}),
		))

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.NoTarget, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7644 Section 3.5.2.2: the values matched by a filter comparing "value" are removed.
	t.Run("removes values of a simple multi-valued attribute matched by a value filter", func(t *testing.T) {
		srv := newTestServer(t)
		id := createWidget(t, srv, &widget{Name: "gizmo", Tags: []any{"red", "blue"}})["id"].(string)

		response := Response(t, srv, patchRequest(t, srv, "Widgets/"+id, patch.Operation{Op: patch.OpRemove, Path: `tags[value eq "red"]`}))

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, []any{"blue"}, ReadBodyAs[widget](t, response).Tags)
	})

	// RFC 7643 Section 7: an immutable attribute SHALL NOT be updated, including by removal.
	t.Run("rejects removing a value of an immutable simple multi-valued attribute", func(t *testing.T) {
		srv := newTestServer(t)
		id := createWidget(t, srv, &widget{Name: "gizmo", Labels: []any{"a", "b"}})["id"].(string)

		response := Response(t, srv, patchRequest(t, srv, "Widgets/"+id, patch.Operation{Op: patch.OpRemove, Path: `labels[value eq "a"]`}))

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.Mutability, ReadBodyAs[scimerrors.Error](t, response).ScimType)
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

	// RFC 7644 Section 3.5.2.3: replacing a value the target already holds SHALL NOT change the modify timestamp; RFC 7643 Section 2.5: null equals unassigned.
	t.Run("a null replace of an absent extension attribute changes nothing", func(t *testing.T) {
		srv := newTestServer(t)
		id, etag := create(t, srv, &core.User{UserName: "bjensen"})

		patched := patchUser(t, srv, id, patch.Operation{Op: patch.OpReplace, Path: string(core.SchemaEnterpriseUser) + ":department", Value: json.RawMessage(`null`)})

		assert.Equal(t, etag, patched.Meta.Version)
		assert.Equal(t, []core.SchemaURI{core.SchemaUser}, patched.Schemas)
	})

	// RFC 7644 Section 3.5.2.3: if no record match was made, the service provider SHALL indicate failure with "noTarget".
	t.Run("rejects a value path that matches nothing with noTarget", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen", Emails: []core.Email{{Value: "a@example.com", Type: "work"}}})

		response := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{Schemas: []core.SchemaURI{protocol.SchemaPatchOp}, Operations: []patch.Operation{
				{Op: patch.OpReplace, Path: `emails[type eq "home"].value`, Value: json.RawMessage(`"b@example.com"`)},
			}}),
		))

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.NoTarget, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7644 Section 3.5.2.2: if a read-only attribute is removed or becomes unassigned, the server SHALL return "mutability".
	t.Run("rejects a replace that unassigns a readOnly sub-attribute", func(t *testing.T) {
		srv := newTestServer(t)
		id := createWidget(t, srv, &widget{Name: "gizmo", Parts: []part{{Serial: "s-1"}}})["id"].(string)

		for _, operation := range []patch.Operation{
			{Op: patch.OpReplace, Path: "parts", Value: json.RawMessage(`[{"serial":"s-1"}]`)},
			{Op: patch.OpReplace, Value: json.RawMessage(`{"parts":[{"serial":"s-1"}]}`)},
		} {
			request := Request(t, srv, http.MethodPatch, basePath+"/Widgets/"+id,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBodyAs(t, protocol.PatchRequest{
					Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
					Operations: []patch.Operation{operation},
				}),
			)
			response := Response(t, srv, request)

			require.Equal(t, http.StatusBadRequest, response.StatusCode, "operation: %s", operation.Value)
			assert.Equal(t, scimerrors.Mutability, ReadBodyAs[scimerrors.Error](t, response).ScimType, "operation: %s", operation.Value)
		}
	})

	// RFC 7644 Section 3.4.2.2: for DateTime types, the comparison is chronological.
	t.Run("matches a value path filter on a dateTime as an instant", func(t *testing.T) {
		srv := newTestServer(t)
		created := createWidget(t, srv, &widget{Name: "gear", Parts: []part{{Serial: "s-1", Built: "2026-01-01T00:00:00Z"}}})

		response := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Widgets/"+created["id"].(string),
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{Schemas: []core.SchemaURI{protocol.SchemaPatchOp}, Operations: []patch.Operation{
				{Op: patch.OpReplace, Path: `parts[built eq "2026-01-01T01:00:00+01:00"].serial`, Value: json.RawMessage(`"s-2"`)},
			}}),
		))

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, []part{{Serial: "s-2", Built: "2026-01-01T00:00:00Z", Inspector: "qa-bot"}}, ReadBodyAs[widget](t, response).Parts)
	})

	// RFC 7644 Section 3.12, Table 9: invalidFilter applies to a PATCH path filter.
	t.Run("rejects a value path filter on an unknown sub-attribute with invalidFilter", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen", Emails: []core.Email{{Value: "a@example.com", Type: "work"}}})

		response := Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, protocol.PatchRequest{Schemas: []core.SchemaURI{protocol.SchemaPatchOp}, Operations: []patch.Operation{
				{Op: patch.OpReplace, Path: `emails[bogus eq "work"].value`, Value: json.RawMessage(`"b@example.com"`)},
			}}),
		))

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidFilter, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})

	// RFC 7643 Section 7: readOnly means "The attribute SHALL NOT be modified."
	t.Run("accepts a pathless replace that repeats the current id", func(t *testing.T) {
		srv := newTestServer(t)
		id := createGroup(t, srv, &core.Group{DisplayName: "Old"}).ID

		rename := func(id, groupID string) *http.Response {
			return Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Groups/"+id,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBodyAs(t, protocol.PatchRequest{Schemas: []core.SchemaURI{protocol.SchemaPatchOp}, Operations: []patch.Operation{
					{Op: patch.OpReplace, Value: json.RawMessage(`{"id":"` + groupID + `","displayName":"New"}`)},
				}}),
			))
		}

		changed := rename(id, "other")
		require.Equal(t, http.StatusBadRequest, changed.StatusCode)
		assert.Equal(t, scimerrors.Mutability, ReadBodyAs[scimerrors.Error](t, changed).ScimType)

		response := rename(id, id)
		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, "New", ReadBodyAs[core.Group](t, response).DisplayName)
	})

	// RFC 7644 Section 3.5.2.3: all matching record values SHALL be replaced, else "noTarget".
	t.Run("replaces values of a simple multi-valued attribute matched by a value filter", func(t *testing.T) {
		for _, tc := range []struct {
			path, value string
			status      int
			scimType    scimerrors.ErrorType
			tags        []any
		}{
			{`tags[value eq "red"]`, `"green"`, http.StatusOK, "", []any{"green", "blue"}},
			{`tags[value eq "red"]`, `"blue"`, http.StatusOK, "", []any{"blue"}},
			{`tags[value eq "pink"]`, `"green"`, http.StatusBadRequest, scimerrors.NoTarget, nil},
			{`labels[value eq "a"]`, `"b"`, http.StatusBadRequest, scimerrors.Mutability, nil},
		} {
			srv := newTestServer(t)
			id := createWidget(t, srv, &widget{Name: "gizmo", Tags: []any{"red", "blue"}, Labels: []any{"a"}})["id"].(string)

			response := Response(t, srv, patchRequest(t, srv, "Widgets/"+id, patch.Operation{Op: patch.OpReplace, Path: tc.path, Value: json.RawMessage(tc.value)}))

			require.Equal(t, tc.status, response.StatusCode, tc.path)
			if tc.status == http.StatusOK {
				assert.Equal(t, tc.tags, ReadBodyAs[widget](t, response).Tags, tc.path)
				continue
			}
			assert.Equal(t, tc.scimType, ReadBodyAs[scimerrors.Error](t, response).ScimType, tc.path)
		}
	})

	// RFC 7644 Section 3.5.2.3: a complex multi-valued attribute holds objects, not a bare value.
	t.Run("rejects a bare value for a complex multi-valued value filter", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen", Emails: []core.Email{{Value: "a@example.com", Type: "work"}}})

		response := Response(t, srv, patchRequest(t, srv, "Users/"+id, patch.Operation{Op: patch.OpReplace, Path: `emails[type eq "work"]`, Value: json.RawMessage(`"x"`)}))

		require.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Equal(t, scimerrors.InvalidValue, ReadBodyAs[scimerrors.Error](t, response).ScimType)
	})
}

// RFC 7644 3.6 Deleting Resources
func TestRFC7644DeletingResources(t *testing.T) {
	// RFC 7644 Section 3.6: the server MUST return 404 for all operations associated with the previously deleted resource.
	t.Run("keeps later resources addressable after deleting an earlier one", func(t *testing.T) {
		srv := newTestServer(t)
		first, _ := create(t, srv, &core.User{UserName: "alice"})
		create(t, srv, &core.User{UserName: "bob"})
		last, _ := create(t, srv, &core.User{UserName: "carol"})

		require.Equal(t, http.StatusNoContent, Response(t, srv, Request(t, srv, http.MethodDelete, basePath+"/Users/"+first, WithBearerToken(validToken))).StatusCode)

		response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users/"+last, WithBearerToken(validToken)))
		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, "carol", ReadBodyAs[core.User](t, response).UserName)

		response = Response(t, srv, Request(t, srv, http.MethodPut, basePath+"/Users/"+last,
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
			WithRequestBodyAs(t, core.User{UserName: "caroline"}),
		))
		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, last, ReadBodyAs[core.User](t, response).ID)
	})

	// RFC 7644 Section 3.6: a successful DELETE SHALL return 204, and operations on the deleted resource MUST return 404.
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

	// RFC 7644 Section 3.14: the client MAY supply an If-Match header so the operation succeeds only if the ETag matches.
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

	// RFC 7644 Section 3.14: without If-Match the delete is unconditional.
	t.Run("deletes without If-Match after a concurrent write", func(t *testing.T) {
		srv := newTestServer(t, withWriteBeforeDelete())
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		response := Response(t, srv, Request(t, srv, http.MethodDelete, basePath+"/Users/"+id, WithBearerToken(validToken)))
		require.Equal(t, http.StatusNoContent, response.StatusCode)

		response = Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users/"+id, WithBearerToken(validToken)))
		assert.Equal(t, http.StatusNotFound, response.StatusCode)
	})

	// RFC 7644 Section 3.14: the delete succeeds only if the If-Match ETag still matches.
	t.Run("rejects an If-Match delete after a concurrent write", func(t *testing.T) {
		srv := newTestServer(t, withWriteBeforeDelete())
		id, etag := create(t, srv, &core.User{UserName: "bjensen"})

		response := Response(t, srv, Request(t, srv, http.MethodDelete, basePath+"/Users/"+id, WithBearerToken(validToken), WithHeader("If-Match", etag)))
		require.Equal(t, http.StatusPreconditionFailed, response.StatusCode)

		response = Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users/"+id, WithBearerToken(validToken)))
		assert.Equal(t, http.StatusOK, response.StatusCode)
	})

	// RFC 7644 Section 3.12: 412 when the update failed because the resource has changed on the server.
	t.Run("rejects a delete with a stale or empty If-Match and leaves the resource intact", func(t *testing.T) {
		for _, ifMatch := range []string{`W/"stale"`, ","} {
			srv := newTestServer(t)
			id, _ := create(t, srv, &core.User{UserName: "bjensen"})

			request := Request(t, srv, http.MethodDelete, basePath+"/Users/"+id,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithHeader("If-Match", ifMatch),
			)
			response := Response(t, srv, request)
			assert.Equal(t, http.StatusPreconditionFailed, response.StatusCode, ifMatch)

			request = Request(t, srv, http.MethodGet, basePath+"/Users/"+id,
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
			)
			response = Response(t, srv, request)
			assert.Equal(t, http.StatusOK, response.StatusCode, ifMatch)
		}
	})

	// RFC 7644 Section 3.12: 404 when the specified resource does not exist.
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
	srv := newTestServer(t)

	response := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Bulk", WithBearerToken(validToken)))

	assert.Equal(t, http.StatusNotImplemented, response.StatusCode)
	assert.Equal(t, "501", ReadBodyAs[scimerrors.Error](t, response).Status)
}

// RFC 7644 3.8 Data Input/Output Formats
func TestRFC7644DataInputOutputFormats(t *testing.T) {
	// RFC 7644 Section 3.8: service providers SHOULD support the header "Accept: application/json".
	t.Run("accepts application/json and answers with application/scim+json", func(t *testing.T) {
		srv := newTestServer(t)

		response := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Users",
			WithBearerToken(validToken),
			WithContentType("application/json"),
			WithAcceptHeader("application/json"),
			WithRequestBodyAs(t, core.User{UserName: "bjensen"}),
		))

		require.Equal(t, http.StatusCreated, response.StatusCode)
		assert.Equal(t, protocol.MediaType, response.Header.Get("Content-Type"))
	})
}

// RFC 7644 3.11 "/Me" Authenticated Subject Alias
func TestRFC7644MeAuthenticatedSubjectAlias(t *testing.T) {
	srv := newTestServer(t)

	// RFC 7644 Section 3.11: a service provider that does NOT support "/Me" SHOULD respond with 501.
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		response := Response(t, srv, Request(t, srv, method, basePath+"/Me", WithBearerToken(validToken)))

		assert.Equal(t, http.StatusNotImplemented, response.StatusCode, method)
		assert.Equal(t, "501", ReadBodyAs[scimerrors.Error](t, response).Status, method)
	}
}

// RFC 7644 3.12 HTTP Status and Error Response Handling
func TestRFC7644HTTPStatusAndErrorResponseHandling(t *testing.T) {
	// RFC 7644 Section 3.12: errors MUST be returned in a JSON body identified by "urn:ietf:params:scim:api:messages:2.0:Error".
	t.Run("returns a SCIM error for an unknown resource id", func(t *testing.T) {
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
	})

	// RFC 7644 Section 3.12: "detail" is an OPTIONAL human-readable message, and "scimType" is a Table 9 keyword.
	t.Run("describes an error with RFC 7644 Table 9 instead of echoing the request", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})
		query := func(key, value string) string { return basePath + "/Users?" + url.Values{key: {value}}.Encode() }
		patch := func(path string) Option[*http.Request] {
			return WithRequestBodyAs(t, protocol.PatchRequest{
				Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{{Op: patch.OpReplace, Path: path, Value: json.RawMessage(`"x"`)}},
			})
		}

		for _, tc := range []struct {
			method, path string
			body         Option[*http.Request]
		}{
			{http.MethodGet, query("filter", `leak eq`), nil},
			{http.MethodGet, query("filter", `leak eq "x"`), nil},
			{http.MethodGet, query("filter", `active eq "leak"`), nil},
			{http.MethodGet, query("filter", `password eq "leak"`), nil},
			{http.MethodGet, query("filter", `LEAKuserName[value eq "x"]`), nil},
			{http.MethodGet, query("filter", `userName[value eq "leak"]`), nil},
			{http.MethodGet, query("filter", `active gt true`), nil},
			{http.MethodGet, query("sortBy", `leak!`), nil},
			{http.MethodGet, query("attributes", `leak!`), nil},
			{http.MethodPost, basePath + "/Users", WithRequestBodyAs(t, core.User{UserName: "leak", UserType: "leak"})},
			{http.MethodPatch, basePath + "/Users/" + id, patch(`leak[`)},
			{http.MethodPatch, basePath + "/Users/" + id, patch(`leak`)},
			{http.MethodPatch, basePath + "/Users/" + id, patch(`urn:leak:User:userName`)},
			{http.MethodPatch, basePath + "/Users/" + id, patch(`emails[leak eq "x"].value`)},
			{http.MethodPatch, basePath + "/Users/" + id, patch(`emails[primary eq "leak"].value`)},
			{http.MethodPatch, basePath + "/Users/" + id, patch(`emails[primary gt true].value`)},
		} {
			options := []Option[*http.Request]{WithBearerToken(validToken), WithContentType(protocol.MediaType)}
			if tc.body != nil {
				options = append(options, tc.body)
			}
			response := Response(t, srv, Request(t, srv, tc.method, tc.path, options...))
			scimErr := ReadBodyAs[scimerrors.Error](t, response)

			require.GreaterOrEqual(t, response.StatusCode, http.StatusBadRequest, tc.path)
			require.NotEmpty(t, scimErr.ScimType, tc.path)
			assert.Equal(t, scimErr.ScimType.Description(), scimErr.Detail, tc.path)
			assert.NotContains(t, strings.ToLower(scimErr.Detail), "leak", tc.path)
		}

		for _, tc := range []struct{ method, path, body string }{
			{http.MethodPost, basePath + "/Users", `{"userName":"leak`},
			{http.MethodPost, basePath + "/Users", `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":{"leak":1}}`},
			{http.MethodPost, basePath + "/Users/.search", `{"schemas":["urn:ietf:params:scim:api:messages:2.0:SearchRequest"],"filter":"leak eq"}`},
			{http.MethodPost, basePath + "/Users/.search", `{"schemas":["urn:ietf:params:scim:api:messages:2.0:SearchRequest"],"sortBy":"leak!"}`},
			{http.MethodPost, basePath + "/Users/.search", `{"schemas":["urn:ietf:params:scim:api:messages:2.0:SearchRequest"],"attributes":["leak!"]}`},
			{http.MethodPatch, basePath + "/Users/" + id, `{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[{"op":"leak","path":"userName","value":"x"}]}`},
			{http.MethodPatch, basePath + "/Users/" + id, `{"schemas":["urn:leak"],"Operations":[{"op":"replace","path":"userName","value":"x"}]}`},
		} {
			response := Response(t, srv, Request(t, srv, tc.method, tc.path, WithBearerToken(validToken), WithContentType(protocol.MediaType), WithRequestBody([]byte(tc.body))))
			scimErr := ReadBodyAs[scimerrors.Error](t, response)

			require.GreaterOrEqual(t, response.StatusCode, http.StatusBadRequest, tc.body)
			assert.NotContains(t, strings.ToLower(scimErr.Detail), "leak", tc.body)
		}
	})

	// RFC 7644 Section 3.12: 404 when the specified resource or endpoint does not exist.
	t.Run("returns a SCIM error for an unknown endpoint", func(t *testing.T) {
		srv := newTestServer(t)

		response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Unknown", WithBearerToken(validToken)))

		require.Equal(t, http.StatusNotFound, response.StatusCode)
		assert.Equal(t, protocol.MediaType, response.Header.Get("Content-Type"))
		assert.Equal(t, "404", ReadBodyAs[scimerrors.Error](t, response).Status)
	})

	// RFC 7231 Section 6.5.5: a 405 response MUST include an Allow header listing the target resource's supported methods.
	t.Run("returns a SCIM error and the allowed methods for an unsupported method", func(t *testing.T) {
		srv := newTestServer(t)

		response := Response(t, srv, Request(t, srv, http.MethodDelete, basePath+"/ServiceProviderConfig", WithBearerToken(validToken)))

		require.Equal(t, http.StatusMethodNotAllowed, response.StatusCode)
		assert.Contains(t, response.Header.Get("Allow"), http.MethodGet)
		assert.Equal(t, protocol.MediaType, response.Header.Get("Content-Type"))
		assert.Equal(t, "405", ReadBodyAs[scimerrors.Error](t, response).Status)
	})
}

// RFC 7644 3.14 Versioning Resources
func TestRFC7644VersioningResources(t *testing.T) {
	// RFC 7644 Section 3.14: service providers MAY support weak ETags as the preferred mechanism.
	t.Run("issues a weak ETag", func(t *testing.T) {
		srv := newTestServer(t)
		_, etag := create(t, srv, &core.User{UserName: "bjensen"})

		assert.Regexp(t, `^W/"[^"]+"$`, etag)
	})

	// RFC 7644 Section 3.14: ETags MUST be an HTTP header and SHOULD be specified within the 'version' attribute of 'meta'.
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

	// RFC 7644 Section 3.14: ETags MUST be an HTTP header and SHOULD be specified within the 'version' attribute of 'meta'.
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

	// RFC 9110 Section 13.1.1: If-Match "*" matches any current representation of the target resource.
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

	// RFC 7644 Section 3.14: if the resource has not changed, the service provider returns an empty body with a 304.
	t.Run("If-None-Match returns 304 only when a listed ETag weakly matches", func(t *testing.T) {
		srv := newTestServer(t)
		id, etag := create(t, srv, &core.User{UserName: "bjensen"})

		for ifNoneMatch, status := range map[string]int{
			etag:                           http.StatusNotModified,
			strings.TrimPrefix(etag, "W/"): http.StatusNotModified,
			`W/"stale", ` + etag:           http.StatusNotModified,
			"*":                            http.StatusNotModified,
			`W/"stale"`:                    http.StatusOK,
		} {
			response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users/"+id,
				WithBearerToken(validToken),
				WithHeader("If-None-Match", ifNoneMatch),
			))
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)

			assert.Equal(t, status, response.StatusCode, ifNoneMatch)
			assert.Equal(t, etag, response.Header.Get("ETag"), ifNoneMatch)
			assert.True(t, strings.HasSuffix(response.Header.Get("Content-Location"), "/Users/"+id), ifNoneMatch)
			assert.Equal(t, status == http.StatusOK, len(body) > 0, ifNoneMatch)
		}
	})

	// RFC 7644 Section 3.14: If-None-Match is a versioning feature, so it is ignored when ETag.Supported is false.
	t.Run("If-None-Match is ignored when ETag.Supported is false", func(t *testing.T) {
		srv := newTestServer(t, withConfig(core.NewServiceProviderConfig().Patching()))
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users/"+id,
			WithBearerToken(validToken),
			WithHeader("If-None-Match", "*"),
		))

		assert.Equal(t, http.StatusOK, response.StatusCode)
	})

	// RFC 7644 Section 3.12: 409 when the specified version number does not match the resource's latest version number.
	t.Run("serves concurrent reads and writes without losing a resource", func(t *testing.T) {
		srv := newTestServer(t)
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		allowed := [][]int{{http.StatusCreated}, {http.StatusOK}, {http.StatusOK}, {http.StatusOK, http.StatusConflict}, {http.StatusOK, http.StatusConflict}}
		requests := make([]*http.Request, 0, 8*len(allowed))
		for i := range 8 {
			requests = append(requests,
				Request(t, srv, http.MethodPost, basePath+"/Users",
					WithBearerToken(validToken),
					WithContentType(protocol.MediaType),
					WithRequestBodyAs(t, core.User{UserName: "user" + strconv.Itoa(i)}),
				),
				Request(t, srv, http.MethodGet, basePath+"/Users/"+id, WithBearerToken(validToken)),
				Request(t, srv, http.MethodGet, basePath+"/Users", WithBearerToken(validToken)),
				Request(t, srv, http.MethodPut, basePath+"/Users/"+id,
					WithBearerToken(validToken),
					WithContentType(protocol.MediaType),
					WithRequestBody([]byte(`{"userName":"bjensen"}`)),
				),
				patchRequest(t, srv, "Users/"+id, activate),
			)
		}
		for i, response := range ConcurrentResponses(t, srv, requests...) {
			assert.Contains(t, allowed[i%len(allowed)], response.StatusCode)
		}

		request := Request(t, srv, http.MethodGet, basePath+"/Users", WithBearerToken(validToken))
		list := ReadBodyAs[protocol.ListResponse[*core.User]](t, Response(t, srv, request))
		assert.Equal(t, 9, list.TotalResults)
	})

	// RFC 7644 Section 3.12: 409 when the specified version number does not match the resource's latest version number.
	t.Run("answers 409 to the loser of two concurrent versionless patches and keeps the winner", func(t *testing.T) {
		srv := newTestServer(t, withUpdateGate(newRaceGate(2)))
		id, _ := create(t, srv, &core.User{UserName: "bjensen"})

		statuses := statusCodes(ConcurrentResponses(t, srv,
			patchRequest(t, srv, "Users/"+id, activate),
			patchRequest(t, srv, "Users/"+id, patch.Operation{Op: patch.OpReplace, Path: "title", Value: json.RawMessage(`"Tour Guide"`)}),
		))

		assert.ElementsMatch(t, []int{http.StatusOK, http.StatusConflict}, statuses)
		user := ReadBodyAs[core.User](t, Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users/"+id, WithBearerToken(validToken))))
		if statuses[0] == http.StatusOK {
			assert.True(t, *user.Active)
			assert.Empty(t, user.Title)
		} else {
			assert.Nil(t, user.Active)
			assert.Equal(t, "Tour Guide", user.Title)
		}
	})

	// RFC 7644 Section 3.14: the repository rejects a write whose If-Match version went stale after the read.
	t.Run("answers 412 to the loser of two concurrent patches with the same If-Match", func(t *testing.T) {
		srv := newTestServer(t, withUpdateGate(newRaceGate(2)))
		id, etag := create(t, srv, &core.User{UserName: "bjensen"})

		statuses := statusCodes(ConcurrentResponses(t, srv,
			patchRequest(t, srv, "Users/"+id, activate, WithHeader("If-Match", etag)),
			patchRequest(t, srv, "Users/"+id, patch.Operation{Op: patch.OpReplace, Path: "title", Value: json.RawMessage(`"Tour Guide"`)}, WithHeader("If-Match", etag)),
		))

		assert.ElementsMatch(t, []int{http.StatusOK, http.StatusPreconditionFailed}, statuses)
	})

	// RFC 7644 Section 3.14: with If-None-Match, an unchanged resource returns an empty body with 304.
	t.Run("retrieves a resource only if it changed with If-None-Match", func(t *testing.T) {
		t.Skip("MAY: conditional retrieval with If-None-Match is not supported")
	})
}

// RFC 7644 4 Service Provider Configuration Endpoints (/ServiceProviderConfig)
func TestRFC7644ServiceProviderConfiguration(t *testing.T) {
	// RFC 7644 Section 4: "/ServiceProviderConfig" describes the SCIM specification features available on a service provider.
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

	// RFC 7643 Section 3.1: meta.location is the URI of the resource being returned.
	t.Run("advertises its own location", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodGet, basePath+"/ServiceProviderConfig", WithBearerToken(validToken))
		config := ReadBodyAs[core.ServiceProviderConfig](t, Response(t, srv, request))

		assert.Equal(t, basePath+"/ServiceProviderConfig", config.Meta.Location)
	})

	// RFC 7643 Section 3.1: meta.location MUST be the same as the "Content-Location" HTTP response header.
	t.Run("keeps a location set by the caller", func(t *testing.T) {
		location := "https://example.com" + basePath + "/ServiceProviderConfig"
		config := fullServiceProviderConfig()
		config.Meta.Location = location
		srv := newTestServer(t, withConfig(config))

		response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/ServiceProviderConfig", WithBearerToken(validToken)))

		assert.Equal(t, location, ReadBodyAs[core.ServiceProviderConfig](t, response).Meta.Location)
		assert.Equal(t, location, response.Header.Get("Content-Location"))
	})

	// RFC 7643 Section 5: filter.maxResults is the maximum number of resources returned in a response.
	t.Run("advertises the custom max results configured via Filtering", func(t *testing.T) {
		config := core.NewServiceProviderConfig().Sorting().Filtering(2).Patching().Versioning()
		srv := newTestServer(t, withConfig(config))

		request := Request(t, srv, http.MethodGet, basePath+"/ServiceProviderConfig", WithBearerToken(validToken))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, 2, ReadBodyAs[core.ServiceProviderConfig](t, response).Filter.MaxResults)
	})

	// RFC 7644 Section 4: "/ServiceProviderConfig" describes the SCIM specification features available on a service provider.
	t.Run("advertises a mutated config directly", func(t *testing.T) {
		config := core.NewServiceProviderConfig().Patching().Filtering(protocol.DefaultLimits.MaxCount)
		config.DocumentationURI = "https://example.com/help/scim.html"
		srv := newTestServer(t, withConfig(config))

		request := Request(t, srv, http.MethodGet, basePath+"/ServiceProviderConfig", WithBearerToken(validToken))
		advertised := ReadBodyAs[core.ServiceProviderConfig](t, Response(t, srv, request))

		assert.Equal(t, "https://example.com/help/scim.html", advertised.DocumentationURI)
		assert.False(t, advertised.Sort.Supported)
		assert.True(t, advertised.Patch.Supported)
	})

	// RFC 7643 Section 5: etag is a complex type that specifies ETag configuration options and is REQUIRED.
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
	// RFC 7644 Section 4: an HTTP GET to "/Schemas" SHALL return all supported schemas in ListResponse format.
	t.Run("lists the registered schemas", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodGet, basePath+"/Schemas",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.Schema]](t, response)
		require.Equal(t, 6, list.TotalResults)
		schema := list.Resources[0]
		assert.Equal(t, core.SchemaUser, schema.ID)
		assert.Equal(t, core.SchemaEnterpriseUser, list.Resources[1].ID)
		assert.Equal(t, core.SchemaGroup, list.Resources[2].ID)
		assert.Equal(t, widgetSchema, list.Resources[3].ID)
		assert.Equal(t, kitSchema, list.Resources[4].ID)
		assert.Equal(t, gadgetSchema, list.Resources[5].ID)

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

	// RFC 7644 Section 4: individual schema definitions can be returned by appending the schema URI to "/Schemas".
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

	// RFC 7643 Section 8.7.2: the schema "id" has "caseExact" false.
	t.Run("fetches a schema by id in another case", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodGet, basePath+"/Schemas/"+strings.ToUpper(string(core.SchemaUser)), WithBearerToken(validToken))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		schema := ReadBodyAs[core.Schema](t, response)
		assert.Equal(t, core.SchemaUser, schema.ID)
		assert.Equal(t, basePath+"/Schemas/"+string(core.SchemaUser), schema.Meta.Location)
	})

	// RFC 7644 Section 3.12: 404 when the specified resource or endpoint does not exist.
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
	// RFC 7643 Section 6: "endpoint" is relative to the Base URL of the service provider, e.g., "Users".
	t.Run("lists the registered resource types", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodGet, basePath+"/ResourceTypes",
			WithBearerToken(validToken),
			WithContentType(protocol.MediaType),
		)
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		list := ReadBodyAs[protocol.ListResponse[*core.ResourceType]](t, response)
		require.Equal(t, 5, list.TotalResults)
		assert.Equal(t, core.ResourceTypeName("User"), list.Resources[0].ID)
		assert.Equal(t, "/Users", list.Resources[0].Endpoint)
		assert.Equal(t, core.SchemaUser, list.Resources[0].Schema)
		assert.Equal(t, core.ResourceTypeName("Group"), list.Resources[1].ID)
		assert.Equal(t, "/Groups", list.Resources[1].Endpoint)
		assert.Equal(t, core.SchemaGroup, list.Resources[1].Schema)
		assert.Equal(t, core.ResourceTypeName("Widget"), list.Resources[2].ID)
		assert.Equal(t, "/Widgets", list.Resources[2].Endpoint)
		assert.Equal(t, widgetSchema, list.Resources[2].Schema)
		assert.Equal(t, core.ResourceTypeName("Kit"), list.Resources[3].ID)
		assert.Equal(t, "/Kits", list.Resources[3].Endpoint)
		assert.Equal(t, kitSchema, list.Resources[3].Schema)
		assert.Equal(t, core.ResourceTypeName("Gadget"), list.Resources[4].ID)
		assert.Equal(t, "/Gadgets", list.Resources[4].Endpoint)
		assert.Equal(t, gadgetSchema, list.Resources[4].Schema)
	})

	// RFC 7644 Section 4: a specific "ResourceType" is returned in the same way that a single User or Group is retrieved.
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
		assert.Equal(t, "/Users", resourceType.Endpoint)
		assert.Equal(t, core.SchemaUser, resourceType.Schema)
	})

	// RFC 7643 Section 8.7.2: the resource type "id" has "caseExact" false.
	t.Run("fetches a resource type by id in another case", func(t *testing.T) {
		srv := newTestServer(t)

		request := Request(t, srv, http.MethodGet, basePath+"/ResourceTypes/user", WithBearerToken(validToken))
		response := Response(t, srv, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, core.ResourceTypeName("User"), ReadBodyAs[core.ResourceType](t, response).ID)
	})

	// RFC 7643 Section 6: "description" is the resource type's human-readable description.
	t.Run("describes a resource type", func(t *testing.T) {
		srv := Server(t, server.New(basePath, fullServiceProviderConfig(),
			server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, userAttributes()...).WithDescription("User Account")),
		))

		response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/ResourceTypes/User"))

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, "User Account", ReadBodyAs[core.ResourceType](t, response).Description)
	})

	// RFC 7644 Section 3.12: 404 when the specified resource or endpoint does not exist.
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

// RFC 7644 7.5.2 Disclosure of Sensitive Information in URIs
func TestRFC7644DisclosureOfSensitiveInformationInURIs(t *testing.T) {
	// RFC 7644 Section 7.5.2: a GET filter that contains sensitive information SHOULD be refused with 403.
	t.Run("rejects a GET filter on a writeOnly attribute with sensitive", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice", Password: "hunter2"})
		createWidget(t, srv, &widget{Name: "gear", Secret: "s3cret", Keys: []map[string]any{{"value": "k3y"}}})

		for _, query := range []struct{ resource, filter string }{
			{"/Users", `password eq "hunter2"`},
			{"/Users", `password sw "hun"`},
			{"/Users", `not (password ne "hunter2")`},
			{"/Users", `userName eq "alice" and password eq "hunter2"`},
			{"/Widgets", `secret eq "s3cret"`},
			{"/Widgets", `keys.value eq "k3y"`},
			{"/Widgets", `keys[value eq "k3y"]`},
		} {
			path := basePath + query.resource + "?" + url.Values{"filter": {query.filter}}.Encode()
			response := Response(t, srv, Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType)))

			require.Equal(t, http.StatusForbidden, response.StatusCode, "filter: %s", query.filter)
			body := ReadBodyAs[scimerrors.Error](t, response)
			assert.Equal(t, scimerrors.Sensitive, body.ScimType, "filter: %s", query.filter)
			assert.NotContains(t, body.Detail, "hunter2", "filter: %s", query.filter)
			assert.NotContains(t, body.Detail, "s3cret", "filter: %s", query.filter)
			assert.NotContains(t, body.Detail, "k3y", "filter: %s", query.filter)
		}
	})

	// RFC 7644 Section 7.5.2: HTTP POST is the remedy for a sensitive filter refused in a GET.
	t.Run("accepts a POST .search filter for equality on a writeOnly attribute", func(t *testing.T) {
		srv := newTestServer(t)
		create(t, srv, &core.User{UserName: "alice", Password: "hunter2"})

		for _, query := range []struct {
			filter string
			status int
		}{
			{`password eq \"hunter2\"`, http.StatusOK},
			{`password sw \"hun\"`, http.StatusBadRequest},
		} {
			body := `{"schemas":["urn:ietf:params:scim:api:messages:2.0:SearchRequest"],"filter":"` + query.filter + `"}`
			response := Response(t, srv, Request(t, srv, http.MethodPost, basePath+"/Users/.search",
				WithBearerToken(validToken),
				WithContentType(protocol.MediaType),
				WithRequestBody([]byte(body)),
			))

			assert.Equal(t, query.status, response.StatusCode, "filter: %s", query.filter)
		}
	})

	// RFC 7643 Section 4.1.1: a password is used to "compare (i.e., filter for equality)".
	t.Run("rejects presence of a writeOnly attribute with invalidFilter because it sends no value", func(t *testing.T) {
		srv := newTestServer(t)

		for _, query := range []struct{ resource, filter string }{
			{"/Users", `password pr`},
			{"/Users", `not (password pr)`},
			{"/Widgets", `keys[value pr]`},
		} {
			path := basePath + query.resource + "?" + url.Values{"filter": {query.filter}}.Encode()
			response := Response(t, srv, Request(t, srv, http.MethodGet, path, WithBearerToken(validToken), WithContentType(protocol.MediaType)))

			require.Equal(t, http.StatusBadRequest, response.StatusCode, "filter: %s", query.filter)
			assert.Equal(t, scimerrors.InvalidFilter, ReadBodyAs[scimerrors.Error](t, response).ScimType, "filter: %s", query.filter)
		}
	})
}
