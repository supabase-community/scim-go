package server_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
	"github.com/supabase-community/scim-go/pkg/server"
)

const basePath = "/scim/v2"

func TestRFC7644(t *testing.T) {
	t.Run("3.3 Creating Resources", func(t *testing.T) {
		t.Run("creates a resource and returns 201 with Location and ETag", func(t *testing.T) {
			ts := newTestServer(t, newUserResource())

			request := Request(t, ts, http.MethodPost, basePath+"/Users",
				WithRequestHeader("Content-Type", protocol.MediaType),
				WithRequestBody([]byte(`{"userName":"bjensen"}`)),
			)
			response := Response(t, ts, request)

			require.Equal(t, http.StatusCreated, response.StatusCode)

			var created core.User
			readJSON(t, response, &created)

			assert.NotEmpty(t, created.ID)
			assert.Equal(t, basePath+"/Users/"+created.ID, response.Header.Get("Location"))
			assert.NotEmpty(t, response.Header.Get("ETag"))
			assert.Equal(t, core.ResourceTypeName("User"), created.Meta.ResourceType)
		})

		t.Run("rejects a malformed JSON body", func(t *testing.T) {
			ts := newTestServer(t, newUserResource())

			response := Response(t, ts, Request(t, ts, http.MethodPost, basePath+"/Users", WithRequestBody([]byte(`{`))))

			require.Equal(t, http.StatusBadRequest, response.StatusCode)
			var scimErr scimerrors.Error
			readJSON(t, response, &scimErr)
			assert.Equal(t, scimerrors.InvalidSyntax, scimErr.ScimType)
		})
	})

	t.Run("3.4.1 Retrieving a Known Resource", func(t *testing.T) {
		t.Run("gets a created resource by id", func(t *testing.T) {
			ts := newTestServer(t, newUserResource())
			id, etag := createUser(t, ts, "bjensen")

			request := Request(t, ts, http.MethodGet, basePath+"/Users/"+id, WithRequestHeader("Content-Type", protocol.MediaType))
			response := Response(t, ts, request)

			require.Equal(t, http.StatusOK, response.StatusCode)
			assert.Equal(t, etag, response.Header.Get("ETag"))
			var got core.User
			readJSON(t, response, &got)
			assert.Equal(t, "bjensen", got.UserName)
		})

		t.Run("returns 404 for an unknown id", func(t *testing.T) {
			ts := newTestServer(t, newUserResource())

			request := Request(t, ts, http.MethodGet, basePath+"/Users/does-not-exist",
				WithRequestHeader("Content-Type", protocol.MediaType),
			)
			response := Response(t, ts, request)

			assert.Equal(t, http.StatusNotFound, response.StatusCode)
		})
	})

	t.Run("3.4.2 Query Resources", func(t *testing.T) {
		t.Run("lists all created resources", func(t *testing.T) {
			ts := newTestServer(t, newUserResource())
			createUser(t, ts, "alice")
			createUser(t, ts, "bob")
			createUser(t, ts, "carol")

			request := Request(t, ts, http.MethodGet, basePath+"/Users",
				WithRequestHeader("Content-Type", protocol.MediaType),
			)
			response := Response(t, ts, request)

			require.Equal(t, http.StatusOK, response.StatusCode)
			var list protocol.ListResponse[*core.User]
			readJSON(t, response, &list)
			assert.Equal(t, 3, list.TotalResults)
		})

		t.Run("paginates with startIndex and count", func(t *testing.T) {
			ts := newTestServer(t, newUserResource())
			createUser(t, ts, "alice")
			createUser(t, ts, "bob")
			createUser(t, ts, "carol")

			path := basePath + "/Users?" + url.Values{"startIndex": {"2"}, "count": {"1"}}.Encode()
			request := Request(t, ts, http.MethodGet, path,
				WithRequestHeader("Content-Type", protocol.MediaType),
			)
			response := Response(t, ts, request)

			require.Equal(t, http.StatusOK, response.StatusCode)
			var list protocol.ListResponse[*core.User]
			readJSON(t, response, &list)
			assert.Equal(t, 3, list.TotalResults)
			assert.Equal(t, 1, list.ItemsPerPage)
			require.Len(t, list.Resources, 1)
			assert.Equal(t, "bob", list.Resources[0].UserName)
		})
	})

	t.Run("3.4.2.2 Filtering", func(t *testing.T) {
		t.Run("filters resources by an exact match on userName", func(t *testing.T) {
			ts := newTestServer(t, newUserResource())
			createUser(t, ts, "alice")
			createUser(t, ts, "bob")

			path := basePath + "/Users?" + url.Values{"filter": {`userName eq "alice"`}}.Encode()
			request := Request(t, ts, http.MethodGet, path,
				WithRequestHeader("Content-Type", protocol.MediaType),
			)
			response := Response(t, ts, request)

			require.Equal(t, http.StatusOK, response.StatusCode)
			var list protocol.ListResponse[*core.User]
			readJSON(t, response, &list)
			require.Equal(t, 1, list.TotalResults)
			assert.Equal(t, "alice", list.Resources[0].UserName)
		})

		t.Run("rejects a filter on an attribute that is not filterable", func(t *testing.T) {
			ts := newTestServer(t, newUserResource())

			path := basePath + "/Users?" + url.Values{"filter": {`bogus eq "x"`}}.Encode()
			request := Request(t, ts, http.MethodGet, path,
				WithRequestHeader("Content-Type", protocol.MediaType),
			)
			response := Response(t, ts, request)

			require.Equal(t, http.StatusBadRequest, response.StatusCode)
			var scimErr scimerrors.Error
			readJSON(t, response, &scimErr)
			assert.Equal(t, scimerrors.InvalidFilter, scimErr.ScimType)
		})
	})

	t.Run("3.4.2.3 Sorting", func(t *testing.T) {
		t.Skip("sortBy/sortOrder are parsed by protocol.ParseSearchRequest but pkg/server/repository.go List never sorts; ServiceProviderConfig still advertises sort.supported=true")
	})

	t.Run("3.4.3 Alternative Query with POST /.search", func(t *testing.T) {
		t.Skip("POST /.search is not registered by server.New; falls through to the generic unknown-path 404")
	})

	t.Run("3.9 Attributes and excludedAttributes", func(t *testing.T) {
		t.Skip("attributes/excludedAttributes are parsed into SearchRequest but never applied to List/Get responses")
	})

	t.Run("3.5.1 Replacing with PUT", func(t *testing.T) {
		t.Run("replaces a resource and returns a new ETag when If-Match matches", func(t *testing.T) {
			ts := newTestServer(t, newUserResource())
			id, etag := createUser(t, ts, "bjensen")

			request := Request(t, ts, http.MethodPut, basePath+"/Users/"+id,
				WithRequestHeader("Content-Type", protocol.MediaType),
				WithRequestHeader("If-Match", etag),
				WithRequestBody([]byte(`{"userName":"bjensen2"}`)),
			)
			response := Response(t, ts, request)

			require.Equal(t, http.StatusOK, response.StatusCode)
			assert.NotEmpty(t, response.Header.Get("ETag"))
			var replaced core.User
			readJSON(t, response, &replaced)
			assert.Equal(t, "bjensen2", replaced.UserName)
		})

		t.Run("the new ETag is not guaranteed to differ from the old one", func(t *testing.T) {
			t.Skip("weakETag (repository.go) formats only whole Unix seconds, so two writes to the same resource within the same wall-clock second produce an identical ETag, silently defeating If-Match-based optimistic concurrency")
		})

		t.Run("replaces a resource even without If-Match", func(t *testing.T) {
			ts := newTestServer(t, newUserResource())
			id, _ := createUser(t, ts, "bjensen")

			request := Request(t, ts, http.MethodPut, basePath+"/Users/"+id,
				WithRequestHeader("Content-Type", protocol.MediaType),
				WithRequestBody([]byte(`{"userName":"bjensen2"}`)),
			)
			response := Response(t, ts, request)

			assert.Equal(t, http.StatusOK, response.StatusCode)
		})

		t.Run("rejects a replace with a stale If-Match", func(t *testing.T) {
			ts := newTestServer(t, newUserResource())
			id, _ := createUser(t, ts, "bjensen")

			request := Request(t, ts, http.MethodPut, basePath+"/Users/"+id,
				WithRequestHeader("Content-Type", protocol.MediaType),
				WithRequestHeader("If-Match", `W/"stale"`),
				WithRequestBody([]byte(`{"userName":"bjensen2"}`)),
			)
			response := Response(t, ts, request)

			assert.Equal(t, http.StatusPreconditionFailed, response.StatusCode)
		})

		t.Run("replacing an unknown id returns 404", func(t *testing.T) {
			ts := newTestServer(t, newUserResource())

			request := Request(t, ts, http.MethodPut, basePath+"/Users/does-not-exist",
				WithRequestHeader("Content-Type", protocol.MediaType),
				WithRequestBody([]byte(`{"userName":"bjensen"}`)),
			)
			response := Response(t, ts, request)

			assert.Equal(t, http.StatusNotFound, response.StatusCode)
		})
	})

	t.Run("3.5.2 Modifying with PATCH", func(t *testing.T) {
		t.Run("patches a resource and returns the updated field with an ETag", func(t *testing.T) {
			ts := newTestServer(t, newUserResource())
			id, _ := createUser(t, ts, "bjensen")

			request := Request(t, ts, http.MethodPatch, basePath+"/Users/"+id,
				WithRequestHeader("Content-Type", protocol.MediaType),
				WithRequestBody(activatePatchBody(t)),
			)
			response := Response(t, ts, request)

			require.Equal(t, http.StatusOK, response.StatusCode)
			assert.NotEmpty(t, response.Header.Get("ETag"))
			var patched core.User
			readJSON(t, response, &patched)
			require.NotNil(t, patched.Active)
			assert.True(t, *patched.Active)
		})

		t.Run("patching an unknown id returns 404", func(t *testing.T) {
			ts := newTestServer(t, newUserResource())

			request := Request(t, ts, http.MethodPatch, basePath+"/Users/does-not-exist",
				WithRequestHeader("Content-Type", protocol.MediaType),
				WithRequestBody(activatePatchBody(t)),
			)
			response := Response(t, ts, request)

			assert.Equal(t, http.StatusNotFound, response.StatusCode)
		})

		t.Run("does not honor If-Match", func(t *testing.T) {
			t.Skip("Controller.Patch never reads the If-Match header; it fetches then replaces using the version it just fetched, so the precondition check always trivially passes, unlike PUT/DELETE (controller.go:86-105)")
		})

		t.Run("does not exercise add/remove operations end-to-end", func(t *testing.T) {
			t.Skip("add/remove ops are unit-tested in pkg/patch but not exercised through the live server")
		})
	})

	t.Run("3.6 Deleting Resources", func(t *testing.T) {
		t.Run("deletes a resource and it is subsequently gone", func(t *testing.T) {
			ts := newTestServer(t, newUserResource())
			id, _ := createUser(t, ts, "bjensen")

			request := Request(t, ts, http.MethodDelete, basePath+"/Users/"+id,
				WithRequestHeader("Content-Type", protocol.MediaType),
			)
			response := Response(t, ts, request)
			require.Equal(t, http.StatusNoContent, response.StatusCode)
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			assert.Empty(t, body)

			getResp := Response(t, ts, Request(t, ts, http.MethodGet, basePath+"/Users/"+id, WithRequestHeader("Content-Type", protocol.MediaType)))
			assert.Equal(t, http.StatusNotFound, getResp.StatusCode)
		})

		t.Run("rejects a delete with a stale If-Match and leaves the resource intact", func(t *testing.T) {
			ts := newTestServer(t, newUserResource())
			id, _ := createUser(t, ts, "bjensen")

			request := Request(t, ts, http.MethodDelete, basePath+"/Users/"+id,
				WithRequestHeader("Content-Type", protocol.MediaType),
				WithRequestHeader("If-Match", `W/"stale"`),
			)
			response := Response(t, ts, request)
			assert.Equal(t, http.StatusPreconditionFailed, response.StatusCode)

			request = Request(t, ts, http.MethodGet, basePath+"/Users/"+id,
				WithRequestHeader("Content-Type", protocol.MediaType),
			)
			response = Response(t, ts, request)
			assert.Equal(t, http.StatusOK, response.StatusCode)
		})

		t.Run("deleting an unknown id returns 404", func(t *testing.T) {
			ts := newTestServer(t, newUserResource())

			request := Request(t, ts, http.MethodDelete, basePath+"/Users/does-not-exist",
				WithRequestHeader("Content-Type", protocol.MediaType),
			)
			response := Response(t, ts, request)
			assert.Equal(t, http.StatusNotFound, response.StatusCode)
		})
	})

	t.Run("3.11 Singular Resource /Me", func(t *testing.T) {
		t.Skip("/Me is not implemented by server.New; requests to it 404 through the generic unknown-path behavior")
	})

	t.Run("3.12 SCIM Errors", func(t *testing.T) {
		ts := newTestServer(t, newUserResource())

		request := Request(t, ts, http.MethodGet, basePath+"/Users/does-not-exist",
			WithRequestHeader("Content-Type", protocol.MediaType),
		)
		response := Response(t, ts, request)

		require.Equal(t, http.StatusNotFound, response.StatusCode)
		assert.Equal(t, protocol.MediaType, response.Header.Get("Content-Type"))

		var scimErr scimerrors.Error
		readJSON(t, response, &scimErr)
		assert.Equal(t, []core.SchemaURI{scimerrors.SchemaError}, scimErr.Schemas)
		assert.Equal(t, "404", scimErr.Status)
		assert.Empty(t, scimErr.ScimType)
		assert.NotEmpty(t, scimErr.Detail)
	})

	t.Run("3.14 ETags", func(t *testing.T) {
		t.Skip("ETag/If-Match work end-to-end for PUT/DELETE, but ServiceProviderConfig never calls .ETag() so etag.supported is false -- the SPC under-claims here, the mirror of the 3.4.2.3 sorting gap which over-claims")
	})

	t.Run("4.1 Service Provider Configuration", func(t *testing.T) {
		ts := newTestServer(t, newUserResource())

		request := Request(t, ts, http.MethodGet, basePath+"/ServiceProviderConfig",
			WithRequestHeader("Content-Type", protocol.MediaType),
		)
		response := Response(t, ts, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		assert.Equal(t, protocol.MediaType, response.Header.Get("Content-Type"))

		var config core.ServiceProviderConfig
		readJSON(t, response, &config)
		assert.True(t, config.Patch.Supported)
		assert.True(t, config.Filter.Supported)
		assert.Equal(t, protocol.DefaultLimits.MaxCount, config.Filter.MaxResults)
		assert.True(t, config.Sort.Supported)
	})

	t.Run("4.2 Resource Types", func(t *testing.T) {
		t.Run("lists the registered resource types", func(t *testing.T) {
			ts := newTestServer(t, newUserResource())

			request := Request(t, ts, http.MethodGet, basePath+"/ResourceTypes",
				WithRequestHeader("Content-Type", protocol.MediaType),
			)
			response := Response(t, ts, request)

			require.Equal(t, http.StatusOK, response.StatusCode)
			var list protocol.ListResponse[*core.ResourceType]
			readJSON(t, response, &list)
			require.Equal(t, 1, list.TotalResults)
			assert.Equal(t, core.ResourceTypeName("User"), list.Resources[0].ID)
			assert.Equal(t, basePath+"/Users", list.Resources[0].Endpoint)
			assert.Equal(t, core.SchemaUser, list.Resources[0].Schema)
		})

		t.Run("fetches a resource type by id", func(t *testing.T) {
			ts := newTestServer(t, newUserResource())

			request := Request(t, ts, http.MethodGet, basePath+"/ResourceTypes/User",
				WithRequestHeader("Content-Type", protocol.MediaType),
			)
			response := Response(t, ts, request)

			assert.Equal(t, http.StatusOK, response.StatusCode)
		})

		t.Run("returns 404 for an unknown resource type id", func(t *testing.T) {
			ts := newTestServer(t, newUserResource())

			request := Request(t, ts, http.MethodGet, basePath+"/ResourceTypes/Bogus",
				WithRequestHeader("Content-Type", protocol.MediaType),
			)
			response := Response(t, ts, request)

			assert.Equal(t, http.StatusNotFound, response.StatusCode)
		})
	})

	t.Run("4.3 Schemas", func(t *testing.T) {
		t.Run("lists the registered schemas", func(t *testing.T) {
			ts := newTestServer(t, newUserResource())

			request := Request(t, ts, http.MethodGet, basePath+"/Schemas",
				WithRequestHeader("Content-Type", protocol.MediaType),
			)
			response := Response(t, ts, request)

			require.Equal(t, http.StatusOK, response.StatusCode)
			var list protocol.ListResponse[*core.Schema]
			readJSON(t, response, &list)
			require.Equal(t, 1, list.TotalResults)
			schema := list.Resources[0]
			assert.Equal(t, core.SchemaUser, schema.ID)

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
			ts := newTestServer(t, newUserResource())

			request := Request(t, ts, http.MethodGet, basePath+"/Schemas/"+string(core.SchemaUser),
				WithRequestHeader("Content-Type", protocol.MediaType),
			)
			response := Response(t, ts, request)

			assert.Equal(t, http.StatusOK, response.StatusCode)
		})

		t.Run("returns 404 for an unknown schema id", func(t *testing.T) {
			ts := newTestServer(t, newUserResource())

			response := execute(t, ts, http.MethodGet, "/Schemas/urn:bogus", nil, nil)
			assert.Equal(t, http.StatusNotFound, response.StatusCode)
		})
	})
}

func TestServerRouting(t *testing.T) {
	t.Run("New with no resources still serves the discovery endpoints", func(t *testing.T) {
		ts := newTestServer(t)

		request := Request(t, ts, http.MethodGet, basePath+"/ServiceProviderConfig",
			WithRequestHeader("Content-Type", protocol.MediaType),
		)
		response := Response(t, ts, request)

		assert.Equal(t, http.StatusOK, response.StatusCode)
	})

	t.Run("mounts every registered resource under the shared base path", func(t *testing.T) {
		ts := newTestServer(t, newUserResource(), newGroupResource())

		request := Request(t, ts, http.MethodGet, basePath+"/ResourceTypes",
			WithRequestHeader("Content-Type", protocol.MediaType),
		)
		response := Response(t, ts, request)

		require.Equal(t, http.StatusOK, response.StatusCode)
		var list protocol.ListResponse[*core.ResourceType]
		readJSON(t, response, &list)

		assert.Equal(t, 2, list.TotalResults)
		var endpoints []string
		for _, resourceType := range list.Resources {
			endpoints = append(endpoints, resourceType.Endpoint)
		}
		assert.ElementsMatch(t, []string{basePath + "/Users", basePath + "/Groups"}, endpoints)
	})

	t.Run("responds method not allowed when the path exists but the verb does not", func(t *testing.T) {
		ts := newTestServer(t, newUserResource())

		request := Request(t, ts, http.MethodDelete, basePath+"/Users",
			WithRequestHeader("Content-Type", protocol.MediaType),
		)
		response := Response(t, ts, request)

		assert.Equal(t, http.StatusMethodNotAllowed, response.StatusCode)
	})

	t.Run("responds not found for a completely unknown path", func(t *testing.T) {
		ts := newTestServer(t, newUserResource())

		request := Request(t, ts, http.MethodGet, basePath+"/Bogus",
			WithRequestHeader("Content-Type", protocol.MediaType),
		)
		response := Response(t, ts, request)
		assert.Equal(t, http.StatusNotFound, response.StatusCode)
	})
}

func newTestServer(t *testing.T, resources ...server.Registration) *httptest.Server {
	t.Helper()

	srv, err := server.New(basePath, resources...)
	require.NoError(t, err)

	return Server(t, srv)
}

func execute(t *testing.T, ts *httptest.Server, method, path string, body []byte, headers map[string]string) *http.Response {
	t.Helper()

	return Response(t, ts, Request(t, ts, method, basePath+path, WithRequestBody(body), WithRequestHeaders(headers)))
}

func readJSON(t *testing.T, response *http.Response, v any) {
	t.Helper()
	require.NoError(t, json.NewDecoder(response.Body).Decode(v))
}

func createUser(t *testing.T, ts *httptest.Server, userName string) (id, etag string) {
	t.Helper()

	response := execute(t, ts, http.MethodPost, "/Users", []byte(`{"userName":"`+userName+`"}`), map[string]string{"Content-Type": protocol.MediaType})
	require.Equal(t, http.StatusCreated, response.StatusCode)

	var created core.User
	readJSON(t, response, &created)
	return created.ID, response.Header.Get("ETag")
}

func activatePatchBody(t *testing.T) []byte {
	t.Helper()

	body, err := json.Marshal(protocol.PatchRequest{
		Schemas: []core.SchemaURI{protocol.SchemaPatchOp},
		Operations: []patch.Operation{
			{Op: patch.OpReplace, Path: "active", Value: json.RawMessage("true")},
		},
	})
	require.NoError(t, err)
	return body
}

func newUserResource() *server.Resource[*core.User] {
	return server.NewResource[*core.User](
		"User",
		"/Users",
		core.SchemaUser,
		userFields(),
	).WithDescription("User Account")
}

func newGroupResource() *server.Resource[*core.Group] {
	return server.NewResource[*core.Group]("Group", "/Groups", core.SchemaGroup, groupFields())
}

func userFields() server.Fields[*core.User] {
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
		server.NewField[*core.User](core.NewAttribute("emails", core.TypeComplex).AsMultiValued(), nil).With(
			server.NewField(core.NewAttribute("value", core.TypeString), func(u *core.User) any {
				values := make([]any, len(u.Emails))
				for i, e := range u.Emails {
					values[i] = e.Value
				}
				return values
			}),
			server.NewField(core.NewAttribute("type", core.TypeString).Suggesting("work", "home", "other"), func(u *core.User) any {
				values := make([]any, len(u.Emails))
				for i, e := range u.Emails {
					values[i] = e.Type
				}
				return values
			}),
			server.NewField(core.NewAttribute("primary", core.TypeBoolean), func(u *core.User) any {
				values := make([]any, len(u.Emails))
				for i, e := range u.Emails {
					values[i] = e.Primary != nil && *e.Primary
				}
				return values
			}),
		),
	)
}

func groupFields() server.Fields[*core.Group] {
	return server.NewFields(
		server.NewField(
			core.NewAttribute("displayName", core.TypeString).AsRequired(),
			func(g *core.Group) any { return g.DisplayName },
		),
	)
}
