package server_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/server"
)

func TestPatchImmutable(t *testing.T) {
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
}

func TestCreateRequired(t *testing.T) {
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
}
