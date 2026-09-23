package server_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/server"
)

type failingWriter struct{ *httptest.ResponseRecorder }

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestErrorHandlerOption(t *testing.T) {
	var reported []error
	srv := server.New(basePath, server.ErrorHandler(func(err error) { reported = append(reported, err) })).
		WithResource(server.NewResource("User", "/Users", core.SchemaUser, userFields()))

	srv.ServeHTTP(failingWriter{httptest.NewRecorder()}, httptest.NewRequest(http.MethodGet, basePath+"/Users/unknown", nil))

	assert.Len(t, reported, 1)
}

func TestWithRepository(t *testing.T) {
	fields := userFields()
	repository := server.NewRepository(basePath+"/Users", core.NewSchema(core.SchemaUser).With(fields.Attributes()...), fields)
	existing, err := repository.Create(t.Context(), &core.User{UserName: "bjensen"})
	require.NoError(t, err)

	srv := Server(t, server.New(basePath).WithResource(
		server.NewResource("User", "/Users", core.SchemaUser, fields).WithRepository(repository),
	))

	response := Response(t, srv, Request(t, srv, http.MethodGet, basePath+"/Users/"+existing.ID))

	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, "bjensen", ReadBodyAs[core.User](t, response).UserName)
}
