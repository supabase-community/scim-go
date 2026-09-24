package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/server"
)

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
}

// RFC 7644 Section 3.12: concurrent creates of the same unique value must not all succeed.
func TestConcurrentUniqueCreates(t *testing.T) {
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
}

type racingRepository struct {
	server.Repository[*core.User]
	races int
}

func (r *racingRepository) Replace(ctx context.Context, item *core.User) (*core.User, error) {
	if r.races > 0 {
		r.races--
		stored, err := r.Get(ctx, item.ID)
		if err != nil {
			return nil, err
		}
		racer := *stored
		racer.UserType = "contractor"
		racer.Meta.Version = ""
		if _, err := r.Repository.Replace(ctx, &racer); err != nil {
			return nil, err
		}
	}
	return r.Repository.Replace(ctx, item)
}

func racingServer(t *testing.T, races int) (*httptest.Server, string, string) {
	t.Helper()
	fields := userFields()
	repository := &racingRepository{
		Repository: server.NewRepository(basePath+"/Users", []*core.Schema{core.NewSchema(core.SchemaUser).With(fields.Attributes()...)}, fields),
	}
	existing, err := repository.Create(t.Context(), &core.User{UserName: "bjensen"})
	require.NoError(t, err)
	repository.races = races
	srv := Server(t, server.New(fullServiceProviderConfig(),
		server.WithResource(server.NewResource("User", "/Users", core.SchemaUser, fields).WithRepository(repository)),
	))
	return srv, existing.ID, existing.Meta.Version
}

func patchActive(t *testing.T, srv *httptest.Server, id string, options ...Option[*http.Request]) *http.Response {
	t.Helper()
	options = append(options,
		WithContentType(protocol.MediaType),
		WithRequestBodyAs(t, protocol.PatchRequest{
			Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
			Operations: []patch.Operation{{Op: patch.OpReplace, Path: "active", Value: json.RawMessage("true")}},
		}),
	)
	return Response(t, srv, Request(t, srv, http.MethodPatch, basePath+"/Users/"+id, options...))
}

// RFC 7644 Section 3.14: without If-Match the server re-applies a PATCH that lost a race instead of losing either write.
func TestPatchRetriesALostRace(t *testing.T) {
	srv, id, _ := racingServer(t, 2)

	response := patchActive(t, srv, id)

	require.Equal(t, http.StatusOK, response.StatusCode)
	patched := ReadBodyAs[core.User](t, response)
	assert.Equal(t, "contractor", patched.UserType)
	require.NotNil(t, patched.Active)
	assert.True(t, *patched.Active)
}

func TestPatchGivesUpAfterRepeatedRaces(t *testing.T) {
	srv, id, _ := racingServer(t, 100)

	assert.Equal(t, http.StatusPreconditionFailed, patchActive(t, srv, id).StatusCode)
}

func TestPatchWithIfMatchReportsALostRace(t *testing.T) {
	srv, id, version := racingServer(t, 1)

	assert.Equal(t, http.StatusPreconditionFailed, patchActive(t, srv, id, WithHeader("If-Match", version)).StatusCode)
}
