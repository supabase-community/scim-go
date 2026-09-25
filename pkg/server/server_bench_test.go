package server_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

func BenchmarkServerUsers(b *testing.B) {
	handler := newTestHandler(b)
	var body []byte
	var location string
	for i := range protocol.DefaultLimits.MaxCount {
		body = benchBody(b, benchUser("seed"+strconv.Itoa(i)))
		location = serve(b, handler, http.MethodPost, basePath+"/Users", body, http.StatusCreated).Header().Get("Location")
	}
	cases := []struct {
		name   string
		method string
		target string
		body   []byte
		status int
	}{
		{"GET", http.MethodGet, location, nil, http.StatusOK},
		{"GET attributes", http.MethodGet, location + "?attributes=userName,emails.value", nil, http.StatusOK},
		{"PUT", http.MethodPut, location, body, http.StatusOK},
		{"PATCH value path", http.MethodPatch, location, benchBody(b, protocol.PatchRequest{
			Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
			Operations: []patch.Operation{{Op: patch.OpReplace, Path: `emails[type eq "work"].value`, Value: json.RawMessage(`"new@example.com"`)}},
		}), http.StatusOK},
		{"LIST 100", http.MethodGet, basePath + "/Users?count=100", nil, http.StatusOK},
		{"LIST 100 filtered and sorted by the repository", http.MethodGet, basePath + `/Users?count=100&sortBy=userName&filter=userName+sw+"seed"`, nil, http.StatusOK},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) { serveEach(b, handler, tc.method, tc.target, tc.body, tc.status) })
	}
	created := 0
	b.Run("POST then DELETE", func(b *testing.B) { createEach(b, handler, &created, true) })
	b.Run("POST", func(b *testing.B) { createEach(b, handler, &created, false) })
}

func createEach(b *testing.B, handler http.Handler, created *int, remove bool) {
	head, tail, _ := bytes.Cut(benchBody(b, benchUser("USERNAME")), []byte("USERNAME"))
	body := []byte{}
	b.ReportAllocs()
	for b.Loop() {
		*created++
		body = append(strconv.AppendInt(append(append(body[:0], head...), "new"...), int64(*created), 10), tail...)
		created := serve(b, handler, http.MethodPost, basePath+"/Users", body, http.StatusCreated)
		if remove {
			serve(b, handler, http.MethodDelete, created.Header().Get("Location"), nil, http.StatusNoContent)
		}
	}
}

func serveEach(b *testing.B, handler http.Handler, method, target string, body []byte, status int) {
	b.ReportAllocs()
	for b.Loop() {
		serve(b, handler, method, target, body, status)
	}
}

func serve(b *testing.B, handler http.Handler, method, target string, body []byte, status int) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+validToken)
	r.Header.Set("Content-Type", protocol.MediaType)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != status {
		b.Fatalf("%s %s: got %d, want %d: %s", method, target, w.Code, status, w.Body)
	}
	return w
}

func benchBody(b *testing.B, value any) []byte {
	body, err := json.Marshal(value)
	if err != nil {
		b.Fatal(err)
	}
	return body
}

func benchUser(userName string) *core.User {
	primary := true
	return &core.User{
		UserName:    userName,
		DisplayName: "Barbara Jensen",
		Name:        core.Name{GivenName: "Barbara", FamilyName: "Jensen"},
		Emails: []core.Email{
			{Value: "work@example.com", Type: "work", Primary: &primary},
			{Value: "home@example.com", Type: "home"},
		},
		PhoneNumbers:   []core.PhoneNumber{{Value: "555-0100", Type: "work"}},
		Addresses:      []core.Address{{StreetAddress: "100 Main St", Locality: "Springfield", Type: "work"}},
		EnterpriseUser: &core.EnterpriseUser{EmployeeNumber: "E100", Department: "Engineering"},
	}
}
