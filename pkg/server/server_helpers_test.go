package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/server"
)

const (
	basePath         = "/scim/v2"
	validToken       = "s3cr3t"
	expiredToken     = "expired"
	unreachableToken = "unreachable"

	widgetSchema core.SchemaURI = "urn:test:widget"
	kitSchema    core.SchemaURI = "urn:test:kit"
	gadgetSchema core.SchemaURI = "urn:test:gadget"
)

var (
	errUnreachable = errors.New("dial tcp 10.0.0.1:5432: connection refused")
	errGateTimeout = errors.New("race gate: not every request arrived")
)

var activate = patch.Operation{Op: patch.OpReplace, Path: "active", Value: json.RawMessage("true")}

var tokens = map[string]error{
	validToken:       nil,
	expiredToken:     fmt.Errorf("%w: expired at noon", server.ErrInvalidToken),
	unreachableToken: errUnreachable,
}

type testServer struct {
	config            *core.ServiceProviderConfig
	options           []server.Option[*server.Server]
	updateGate        *raceGate
	writeBeforeDelete bool
	user              core.Attributes
	enterprise        core.Attributes
}

type raceGate struct {
	mu      sync.Mutex
	arrived int
	total   int
	release chan struct{}
}

func newRaceGate(total int) *raceGate {
	return &raceGate{total: total, release: make(chan struct{})}
}

func (g *raceGate) arrive() error {
	g.mu.Lock()
	g.arrived++
	last := g.arrived == g.total
	g.mu.Unlock()
	if last {
		close(g.release)
	}
	select {
	case <-g.release:
		return nil
	case <-time.After(5 * time.Second):
		return errGateTimeout
	}
}

type testOption func(*testServer)

type widget struct {
	core.Base
	Name   string
	Score  int64
	When   time.Time
	Nick   string
	Tags   []any
	Parts  []part
	Secret string           `json:",omitempty"`
	Keys   []map[string]any `json:",omitempty"`
	Note   string           `json:",omitempty"`
	Labels []any            `json:",omitempty"`
	Weight float64          `json:",omitempty"`
}

type kit struct {
	core.Base
	Active *bool
	Name   *core.Name
	Parts  []part
}

type part struct {
	Serial    string
	Built     string `json:",omitempty"`
	Code      string `json:",omitempty"`
	Inspector string `json:",omitempty"`
}

type gadget struct {
	core.Base
	Parts []part
}

type gatedRepository struct {
	server.Repository[*core.User]
	gate              *raceGate
	writeBeforeDelete bool
}

type inspectingRepository struct {
	server.Repository[*widget]
}

type projectingRepository[T core.Resource] struct {
	server.Repository[T]
	attribute string
	reads     []bool
	writes    []bool
	updated   protocol.Projection
}

type countingRepository struct {
	server.Repository[*core.User]
	reads int
}

func userSchemas() core.Schemas {
	return core.Schemas{core.NewSchema(core.SchemaUser).WithName("User").With(core.UserAttributes()...)}
}

func groupSchemas() core.Schemas {
	return core.Schemas{core.NewSchema(core.SchemaGroup).WithName("Group").With(core.GroupAttributes()...)}
}

func newGroupHandler(tb testing.TB, repo server.Repository[*core.Group]) http.Handler {
	tb.Helper()
	return server.New(basePath, fullServiceProviderConfig(),
		server.WithAuthentication(core.NewOAuthBearerToken().AsPrimary(), server.RequireBearerToken(validate)),
		server.WithResource(server.NewResource[*core.Group]("Group", "/Groups", core.SchemaGroup, core.GroupAttributes()...).WithRepository(repo)),
	)
}

func newTestServer(t *testing.T, options ...testOption) *httptest.Server {
	t.Helper()

	return Server(t, newTestHandler(t, options...))
}

func newTestHandler(tb testing.TB, options ...testOption) http.Handler {
	tb.Helper()

	s := &testServer{config: fullServiceProviderConfig(), user: core.UserAttributes(), enterprise: core.EnterpriseUserAttributes()}
	for _, option := range options {
		option(s)
	}
	users := gatedRepository{
		Repository: server.NewRepository[*core.User](basePath+"/Users", core.Schemas{
			core.NewSchema(core.SchemaUser).WithName("User").With(s.user...),
			core.NewSchema(core.SchemaEnterpriseUser).With(s.enterprise...),
		}),
		gate:              s.updateGate,
		writeBeforeDelete: s.writeBeforeDelete,
	}
	standard := []server.Option[*server.Server]{
		server.ErrorHandler(func(_ *http.Request, err error) {
			if !errors.Is(err, errUnreachable) {
				tb.Errorf("%v\n", err)
			}
		}),
		server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, s.user...).
			WithExtension(core.SchemaEnterpriseUser, s.enterprise...).
			WithRepository(users)),
		server.WithResource(server.NewResource[*core.Group]("Group", "/Groups", core.SchemaGroup, core.GroupAttributes()...)),
		server.WithResource(server.NewResource[*widget]("Widget", "/Widgets", widgetSchema, widgetAttributes()...).
			WithRepository(inspectingRepository{server.NewRepository[*widget](basePath+"/Widgets", core.Schemas{
				core.NewSchema(widgetSchema).WithName("Widget").With(widgetAttributes()...),
			})})),
		server.WithResource(server.NewResource[*kit]("Kit", "/Kits", kitSchema, kitAttributes()...)),
		server.WithResource(server.NewResource[*gadget]("Gadget", "/Gadgets", gadgetSchema, gadgetAttributes()...)),
		server.WithAuthentication(core.NewOAuthBearerToken().AsPrimary(), server.RequireBearerToken(validate)),
	}
	return server.New(basePath, s.config, slices.Concat(standard, s.options)...)
}

func withConfig(config *core.ServiceProviderConfig) testOption {
	return func(s *testServer) { s.config = config }
}

func withOption(options ...server.Option[*server.Server]) testOption {
	return func(s *testServer) { s.options = append(s.options, options...) }
}

func withUpdateGate(gate *raceGate) testOption {
	return func(s *testServer) { s.updateGate = gate }
}

func withWriteBeforeDelete() testOption {
	return func(s *testServer) { s.writeBeforeDelete = true }
}

func withUserAttributes(change func(user, enterprise core.Attributes)) testOption {
	return func(s *testServer) { change(s.user, s.enterprise) }
}

func canonicalUserType(user, _ core.Attributes) {
	user.Lookup("userType").Suggesting("employee", "contractor")
}

func immutableFamilyName(user, _ core.Attributes) {
	user.Lookup("name").SubAttribute("familyName").AsImmutable()
}

func immutableEmployeeNumber(_, enterprise core.Attributes) {
	enterprise.Lookup("employeeNumber").AsImmutable()
}

func (r gatedRepository) Update(ctx context.Context, user *core.User) (*core.User, error) {
	if r.gate != nil {
		if err := r.gate.arrive(); err != nil {
			return nil, err
		}
	}
	return r.Repository.Update(ctx, user)
}

func (r gatedRepository) Delete(ctx context.Context, user *core.User) error {
	if r.writeBeforeDelete {
		renamed := &core.User{UserName: "renamed"}
		renamed.ID = user.ID
		if _, err := r.Repository.Update(ctx, renamed); err != nil {
			return err
		}
	}
	return r.Repository.Delete(ctx, user)
}

func (r inspectingRepository) Create(ctx context.Context, w *widget) (*widget, error) {
	for i := range w.Parts {
		w.Parts[i].Inspector = "qa-bot"
	}
	return r.Repository.Create(ctx, w)
}

func (r *projectingRepository[T]) Read(ctx context.Context, id string) (T, error) {
	r.reads = r.record(ctx, r.reads)
	return r.Repository.Read(ctx, id)
}

func (r *projectingRepository[T]) List(ctx context.Context, query *protocol.SearchRequest) ([]T, int, error) {
	r.reads = r.record(ctx, r.reads)
	return r.Repository.List(ctx, query)
}

func (r *projectingRepository[T]) Create(ctx context.Context, item T) (T, error) {
	r.writes = r.record(ctx, r.writes)
	return r.Repository.Create(ctx, item)
}

func (r *projectingRepository[T]) Update(ctx context.Context, item T) (T, error) {
	r.writes = r.record(ctx, r.writes)
	r.updated = protocol.ProjectionFrom(ctx)
	return r.Repository.Update(ctx, item)
}

func (r *projectingRepository[T]) record(ctx context.Context, seen []bool) []bool {
	return append(seen, protocol.ProjectionFrom(ctx).Returns(r.attribute))
}

func (r *countingRepository) Read(ctx context.Context, id string) (*core.User, error) {
	r.reads++
	return r.Repository.Read(ctx, id)
}

func validate(ctx context.Context, token string) (context.Context, error) {
	err, ok := tokens[token]
	if !ok {
		return ctx, server.ErrInvalidToken
	}
	return ctx, err
}

func fullServiceProviderConfig() *core.ServiceProviderConfig {
	return core.NewServiceProviderConfig().Sorting().Filtering(protocol.DefaultLimits.MaxCount).Patching().Versioning()
}

func widgetAttributes() core.Attributes {
	return core.Attributes{
		core.NewAttribute("name", core.TypeString).AsRequired().UniqueOn(core.UniquenessServer),
		core.NewAttribute("score", core.TypeInteger),
		core.NewAttribute("when", core.TypeDateTime),
		core.NewAttribute("nick", core.TypeString).UniqueOn(core.UniquenessServer).AsCaseExact(),
		core.NewAttribute("tags", core.TypeString).AsMultiValued(),
		core.NewAttribute("parts", core.TypeComplex).AsMultiValued().With(
			core.NewAttribute("serial", core.TypeString).AsRequired(),
			core.NewAttribute("built", core.TypeDateTime),
			core.NewAttribute("inspector", core.TypeString).AsReadOnly(),
			core.NewAttribute("code", core.TypeString).AsWriteOnly(),
		),
		core.NewAttribute("secret", core.TypeString).AsWriteOnly(),
		core.NewAttribute("keys", core.TypeComplex).AsMultiValued().AsWriteOnly().With(core.NewAttribute("value", core.TypeString)),
		core.NewAttribute("note", core.TypeString).ReturnedAs(core.ReturnedRequest),
		core.NewAttribute("labels", core.TypeString).AsMultiValued().AsImmutable(),
		core.NewAttribute("weight", core.TypeDecimal).AsImmutable(),
	}
}

func kitAttributes() core.Attributes {
	return core.Attributes{
		core.NewAttribute("active", core.TypeBoolean).AsRequired(),
		core.NewAttribute("name", core.TypeComplex).AsRequired().With(core.NewAttribute("givenName", core.TypeString)),
		core.NewAttribute("parts", core.TypeComplex).AsMultiValued().AsRequired().With(
			core.NewAttribute("serial", core.TypeString).AsRequired(),
		),
	}
}

func gadgetAttributes() core.Attributes {
	return core.Attributes{
		core.NewAttribute("parts", core.TypeComplex).AsMultiValued().With(
			core.NewAttribute("serial", core.TypeString).AsRequired().ReturnedAs(core.ReturnedAlways),
			core.NewAttribute("code", core.TypeString).AsImmutable(),
		),
	}
}

func create(t *testing.T, srv *httptest.Server, user *core.User) (id, etag string) {
	t.Helper()

	request := Request(t, srv, http.MethodPost, basePath+"/Users",
		WithBearerToken(validToken),
		WithAcceptHeader(protocol.MediaType),
		WithContentType(protocol.MediaType),
		WithRequestBodyAs(t, user),
	)
	response := Response(t, srv, request)
	require.Equal(t, http.StatusCreated, response.StatusCode)

	created := ReadBodyAs[core.User](t, response)
	return created.ID, response.Header.Get("ETag")
}

func createGroup(t *testing.T, srv *httptest.Server, group *core.Group) core.Group {
	t.Helper()

	request := Request(t, srv, http.MethodPost, basePath+"/Groups",
		WithBearerToken(validToken),
		WithContentType(protocol.MediaType),
		WithRequestBodyAs(t, group),
	)
	response := Response(t, srv, request)
	require.Equal(t, http.StatusCreated, response.StatusCode)
	return ReadBodyAs[core.Group](t, response)
}

func createWidget(t *testing.T, srv *httptest.Server, w *widget) map[string]any {
	t.Helper()

	request := Request(t, srv, http.MethodPost, basePath+"/Widgets",
		WithBearerToken(validToken),
		WithContentType(protocol.MediaType),
		WithRequestBodyAs(t, w),
	)
	response := Response(t, srv, request)
	require.Equal(t, http.StatusCreated, response.StatusCode)
	return ReadBodyAs[map[string]any](t, response)
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

func patchRequest(t *testing.T, srv *httptest.Server, resource string, op patch.Operation, options ...Option[*http.Request]) *http.Request {
	t.Helper()

	return Request(t, srv, http.MethodPatch, basePath+"/"+resource, append(options,
		WithBearerToken(validToken),
		WithContentType(protocol.MediaType),
		WithRequestBodyAs(t, protocol.PatchRequest{Schemas: []core.SchemaURI{protocol.SchemaPatchOp}, Operations: []patch.Operation{op}}),
	)...)
}

func assertFilterMatches(t *testing.T, srv *httptest.Server, filter, userName string) {
	t.Helper()

	path := basePath + "/Users?" + url.Values{"filter": {filter}}.Encode()
	response := Response(t, srv, Request(t, srv, http.MethodGet, path, WithBearerToken(validToken)))
	require.Equal(t, http.StatusOK, response.StatusCode, filter)
	list := ReadBodyAs[protocol.ListResponse[*core.User]](t, response)
	if userName == "" {
		assert.Zero(t, list.TotalResults, filter)
		return
	}
	require.Equal(t, 1, list.TotalResults, filter)
	assert.Equal(t, userName, list.Resources[0].UserName, filter)
}

func swapCase(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsUpper(r) {
			return unicode.ToLower(r)
		}
		return unicode.ToUpper(r)
	}, s)
}
