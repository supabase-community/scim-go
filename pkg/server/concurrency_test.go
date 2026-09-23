package server_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/protocol"
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
			assert.Equal(t, http.StatusOK, Response(t, srv, request).StatusCode)
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
