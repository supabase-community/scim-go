package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/server"
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

func BenchmarkServerPatchManyAddresses(b *testing.B) {
	handler := newTestHandler(b)
	for _, n := range []int{5000, 10000} {
		first := patchAddresses(n, "a")
		second := patchAddresses(n, "b")
		body := benchBody(b, protocol.PatchRequest{
			Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
			Operations: []patch.Operation{first, second},
		})
		location := serve(b, handler, http.MethodPost, basePath+"/Users", benchBody(b, benchUser("addr"+strconv.Itoa(n))), http.StatusCreated).Header().Get("Location")
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			serveEach(b, handler, http.MethodPatch, location, body, http.StatusOK)
		})
	}
}

func BenchmarkServerPatchGroupMembers(b *testing.B) {
	benchmarkPatchGroupMembers(b, newTestHandler(b))
}

func BenchmarkServerPatchGroupMembersFastPathFloor(b *testing.B) {
	benchmarkPatchGroupMembers(b, newGroupHandler(b, stubDeltaRepository{Repository: server.NewRepository[*core.Group](basePath+"/Groups", groupSchemas())}))
}

func BenchmarkServerPutGroupMembers(b *testing.B) {
	handler := newTestHandler(b)
	group := benchGroup(10000, "seed")
	location := serve(b, handler, http.MethodPost, basePath+"/Groups", benchBody(b, group), http.StatusCreated).Header().Get("Location")
	body := benchBody(b, group)
	b.ReportAllocs()
	for b.Loop() {
		serve(b, handler, http.MethodPut, location, body, http.StatusOK)
	}
}

func BenchmarkServerUpdateLockedWork(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		repo := &timedGroupRepository{Repository: server.NewRepository[*core.Group](basePath+"/Groups", groupSchemas())}
		handler := newGroupHandler(b, repo)
		group := benchGroup(n, "seed")
		location := serve(b, handler, http.MethodPost, basePath+"/Groups", benchBody(b, group), http.StatusCreated).Header().Get("Location")
		rename := func(name string) []byte {
			return benchBody(b, protocol.PatchRequest{
				Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
				Operations: []patch.Operation{{Op: patch.OpReplace, Path: "displayName", Value: json.RawMessage(strconv.Quote(name))}},
			})
		}
		put := benchBody(b, group)
		for _, c := range []struct {
			name   string
			method string
			bodies [2][]byte
		}{
			{"PUT", http.MethodPut, [2][]byte{put, put}},
			{"PATCH displayName", http.MethodPatch, [2][]byte{rename("engineering"), rename("platform")}},
		} {
			b.Run(c.name+"/"+strconv.Itoa(n), func(b *testing.B) {
				repo.held = 0
				b.ReportAllocs()
				for i := 0; b.Loop(); i++ {
					serve(b, handler, c.method, location, c.bodies[i%2], http.StatusOK)
				}
				b.ReportMetric(float64(repo.held.Nanoseconds())/float64(b.N), "locked-ns/op")
			})
		}
	}
}

type timedGroupRepository struct {
	server.Repository[*core.Group]
	held time.Duration
}

func (r *timedGroupRepository) Update(ctx context.Context, id, version string, change func(*core.Group) (*core.Group, error)) (*core.Group, error) {
	return r.Repository.Update(ctx, id, version, func(current *core.Group) (*core.Group, error) {
		start := time.Now()
		defer func() { r.held += time.Since(start) }()
		return change(current)
	})
}

func benchmarkPatchGroupMembers(b *testing.B, handler http.Handler) {
	for _, n := range []int{100, 1000, 10000} {
		location := serve(b, handler, http.MethodPost, basePath+"/Groups", benchBody(b, benchGroup(n, "seed"+strconv.Itoa(n))), http.StatusCreated).Header().Get("Location")
		add := benchBody(b, protocol.PatchRequest{
			Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
			Operations: []patch.Operation{{Op: patch.OpAdd, Path: "members", Value: json.RawMessage(`{"value":"toggle","$ref":"https://example.com/Users/toggle","type":"User"}`)}},
		})
		remove := benchBody(b, protocol.PatchRequest{
			Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
			Operations: []patch.Operation{{Op: patch.OpRemove, Path: `members[value eq "toggle"]`}},
		})
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; b.Loop(); i++ {
				body := add
				if i%2 == 1 {
					body = remove
				}
				serve(b, handler, http.MethodPatch, location, body, http.StatusNoContent)
			}
		})
	}
}

func benchGroup(n int, prefix string) *core.Group {
	members := make([]core.Member, n)
	for i := range members {
		value := prefix + strconv.Itoa(i)
		members[i] = core.Member{Value: value, Ref: "https://example.com/Users/" + value, Type: "User"}
	}
	return &core.Group{DisplayName: "bench", Members: members}
}

func patchAddresses(n int, prefix string) patch.Operation {
	elements := make([]map[string]any, n)
	for i := range elements {
		elements[i] = map[string]any{"postalCode": prefix + strconv.Itoa(i)}
	}
	raw, _ := json.Marshal(elements)
	return patch.Operation{Op: patch.OpAdd, Path: "addresses", Value: raw}
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
