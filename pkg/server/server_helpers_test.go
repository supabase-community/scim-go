package server_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
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

var errUnreachable = errors.New("dial tcp 10.0.0.1:5432: connection refused")

var tokens = map[string]error{
	validToken:       nil,
	expiredToken:     fmt.Errorf("%w: expired at noon", server.ErrInvalidToken),
	unreachableToken: errUnreachable,
}

type testServer struct {
	config      *core.ServiceProviderConfig
	options     []server.Option[*server.Server]
	replaceGate *raceGate
	races       *races
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

func (g *raceGate) arrive() {
	g.mu.Lock()
	g.arrived++
	last := g.arrived == g.total
	g.mu.Unlock()
	if last {
		close(g.release)
	}
	<-g.release
}

type races struct {
	mu       sync.Mutex
	lose     int
	replaces int
}

func alwaysLosing() *races {
	return &races{lose: math.MaxInt}
}

func (r *races) replace() (lost bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.replaces++
	if r.lose > 0 {
		r.lose--
		return true
	}
	return false
}

func (r *races) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.replaces
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

type racingRepository struct {
	server.Repository[*core.User]
	replaceGate *raceGate
	races       *races
}

type inspectingRepository struct {
	server.Repository[*widget]
}

type projectingRepository struct {
	server.Repository[*core.User]
	returned []bool
}

type countingRepository struct {
	server.Repository[*core.User]
	gets int
}

type countingRepo interface {
	server.Repository[*core.Group]
	Gets() int
}

type countingGroupRepository struct {
	server.Repository[*core.Group]
	gets int
}

type countingGroupDeltaRepository struct {
	server.Repository[*core.Group]
	gets int
}

type stubDeltaRepository struct {
	server.Repository[*core.Group]
}

func (stubDeltaRepository) PatchAttribute(context.Context, string, string, server.AttributeDelta) (core.Meta, bool, error) {
	return core.Meta{Version: `W/"stub"`}, true, nil
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

	s := &testServer{config: fullServiceProviderConfig(), races: &races{}}
	for _, option := range options {
		option(s)
	}
	users := racingRepository{
		Repository: server.NewRepository[*core.User](basePath+"/Users", core.Schemas{
			core.NewSchema(core.SchemaUser).WithName("User").With(userAttributes()...),
			core.NewSchema(core.SchemaEnterpriseUser).With(enterpriseAttributes()...),
		}),
		replaceGate: s.replaceGate,
		races:       s.races,
	}
	standard := []server.Option[*server.Server]{
		server.ErrorHandler(func(_ *http.Request, err error) {
			if !errors.Is(err, errUnreachable) {
				tb.Errorf("%v\n", err)
			}
		}),
		server.WithResource(server.NewResource[*core.User]("User", "/Users", core.SchemaUser, userAttributes()...).
			WithExtension(core.SchemaEnterpriseUser, enterpriseAttributes()...).
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

func withReplaceGate(gate *raceGate) testOption {
	return func(s *testServer) { s.replaceGate = gate }
}

func withRaces(r *races) testOption {
	return func(s *testServer) { s.races = r }
}

func (r racingRepository) Replace(ctx context.Context, user *core.User) (*core.User, error) {
	if r.races.replace() {
		return nil, scimerrors.ErrPreconditionFailed("resource has changed on the server")
	}
	if r.replaceGate != nil {
		r.replaceGate.arrive()
	}
	return r.Repository.Replace(ctx, user)
}

func (r inspectingRepository) Create(ctx context.Context, w *widget) (*widget, error) {
	for i := range w.Parts {
		w.Parts[i].Inspector = "qa-bot"
	}
	return r.Repository.Create(ctx, w)
}

func (r *projectingRepository) Get(ctx context.Context, id string) (*core.User, error) {
	r.record(ctx)
	return r.Repository.Get(ctx, id)
}

func (r *projectingRepository) List(ctx context.Context, query *protocol.SearchRequest) ([]*core.User, int, error) {
	r.record(ctx)
	return r.Repository.List(ctx, query)
}

func (r *projectingRepository) record(ctx context.Context) {
	r.returned = append(r.returned, protocol.ProjectionFrom(ctx).Returns("emails"))
}

func (r *countingRepository) Get(ctx context.Context, id string) (*core.User, error) {
	r.gets++
	return r.Repository.Get(ctx, id)
}

func (r *countingGroupRepository) Get(ctx context.Context, id string) (*core.Group, error) {
	r.gets++
	return r.Repository.Get(ctx, id)
}

func (r *countingGroupRepository) Gets() int { return r.gets }

func (r *countingGroupDeltaRepository) Get(ctx context.Context, id string) (*core.Group, error) {
	r.gets++
	return r.Repository.Get(ctx, id)
}

func (r *countingGroupDeltaRepository) Gets() int { return r.gets }

func (r *countingGroupDeltaRepository) PatchAttribute(ctx context.Context, id, version string, delta server.AttributeDelta) (core.Meta, bool, error) {
	patcher := r.Repository.(server.AttributePatcher[*core.Group])
	return patcher.PatchAttribute(ctx, id, version, delta)
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

func userAttributes() core.Attributes {
	attributes := core.UserAttributes()
	attributes.Lookup("name").SubAttribute("familyName").AsImmutable()
	attributes.Lookup("userType").Suggesting("employee", "contractor")
	return attributes
}

func enterpriseAttributes() core.Attributes {
	return core.Attributes{
		core.NewAttribute("employeeNumber", core.TypeString).AsImmutable(),
		core.NewAttribute("department", core.TypeString),
		core.NewAttribute("manager", core.TypeComplex).With(
			core.NewAttribute("value", core.TypeString),
			core.NewAttribute("displayName", core.TypeString).AsReadOnly(),
		),
	}
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
			core.NewAttribute("serial", core.TypeString).AsRequired(),
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

func patchActive(t *testing.T, srv *httptest.Server, id string, options ...Option[*http.Request]) *http.Request {
	t.Helper()

	return Request(t, srv, http.MethodPatch, basePath+"/Users/"+id, append(options,
		WithBearerToken(validToken),
		WithContentType(protocol.MediaType),
		WithRequestBodyAs(t, protocol.PatchRequest{
			Schemas:    []core.SchemaURI{protocol.SchemaPatchOp},
			Operations: []patch.Operation{{Op: patch.OpReplace, Path: "active", Value: json.RawMessage("true")}},
		}),
	)...)
}
