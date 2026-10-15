package scimtest

import (
	"net/http"
	"slices"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

type Suite struct {
	client   *Client
	fixtures map[core.ResourceTypeName]fixture
	strict   bool
}

type discovery struct {
	config  core.ServiceProviderConfig
	kinds   []core.ResourceType
	schemas core.Schemas
}

type section struct {
	name  string
	check func(r *resource, t *testing.T)
}

var sections = []section{
	{"3.3 Creating Resources", (*resource).checkCreate},
	{"3.4.1 Retrieving a Known Resource", (*resource).checkRead},
	{"3.4.2 Query Resources", (*resource).checkList},
	{"3.4.2.2 Filtering", (*resource).checkFilter},
	{"3.4.2.3 Sorting", (*resource).checkSort},
	{"3.4.2.4 Pagination", (*resource).checkPagination},
	{"3.4.2.5 Attributes", (*resource).checkAttributes},
	{"3.4.3 Querying Resources Using HTTP POST", (*resource).checkSearch},
	{"3.5.1 Replacing with PUT", (*resource).checkReplace},
	{"3.5.2 Modifying with PATCH", (*resource).checkPatch},
	{"3.6 Deleting Resources", (*resource).checkDelete},
	{"3.14 Versioning Resources", (*resource).checkVersioning},
}

func NewSuite(c *Client, opts ...Option[*Suite]) *Suite {
	s := &Suite{client: c, fixtures: defaultFixtures()}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func WithFixture[T core.Resource](resourceType string, fn func(unique string) T) Option[*Suite] {
	return func(s *Suite) {
		s.fixtures[core.ResourceTypeName(resourceType)] = func(_ *core.Schema, unique string) (core.Object, error) {
			return core.NewObject(fn(unique))
		}
	}
}

// Strict fails todo skips, runs SHOULD checks, and expects 501 for unsupported features, per RFC 7644, Table 8.
func Strict() Option[*Suite] {
	return func(s *Suite) { s.strict = true }
}

func (s *Suite) Run(t *testing.T) {
	t.Helper()

	d := s.discover(t)
	resources := s.resources(d)
	t.Run("RFC7644", func(t *testing.T) {
		t.Run("4 Service Provider Configuration Endpoints", func(t *testing.T) { s.checkDiscovery(t, d) })
		for _, sec := range sections {
			t.Run(sec.name, func(t *testing.T) {
				for _, r := range resources {
					t.Run(string(r.kind.Name), func(t *testing.T) { sec.check(r, t) })
				}
			})
		}
		t.Run("3.7 Bulk Operations", func(t *testing.T) { s.checkBulk(t, &d.config) })
	})
}

func (s *Suite) discover(t *testing.T) *discovery {
	t.Helper()

	res := s.client.Get(t, "/ServiceProviderConfig")
	expectStatus(t, res, http.StatusOK, "RFC 7644 4")
	return &discovery{
		config:  Decode[core.ServiceProviderConfig](t, res),
		kinds:   list[core.ResourceType](t, s.client, "/ResourceTypes").Resources,
		schemas: list[*core.Schema](t, s.client, "/Schemas").Resources,
	}
}

func (s *Suite) resources(d *discovery) []*resource {
	resources := make([]*resource, 0, len(d.kinds))
	for _, kind := range d.kinds {
		if schema := d.schemas.Lookup(kind.Schema); schema != nil {
			resources = append(resources, &resource{suite: s, config: &d.config, kind: kind, schema: schema, fixture: s.fixtures[kind.Name]})
		}
	}
	return resources
}

func (s *Suite) checkDiscovery(t *testing.T, d *discovery) {
	t.Helper()

	if !slices.Contains(d.config.Schemas, core.SchemaServiceProviderConfig) {
		t.Errorf("scimtest: GET /ServiceProviderConfig (RFC 7643 5): schemas %v, want %s", d.config.Schemas, core.SchemaServiceProviderConfig)
	}
	for _, kind := range d.kinds {
		uris := []core.SchemaURI{kind.Schema}
		for _, extension := range kind.SchemaExtensions {
			uris = append(uris, extension.Schema)
		}
		for _, uri := range uris {
			if d.schemas.Lookup(uri) == nil {
				t.Errorf("scimtest: GET /Schemas (RFC 7643 6): %s of resource type %s is missing", uri, kind.Name)
			}
		}
	}
}

func (s *Suite) checkBulk(t *testing.T, config *core.ServiceProviderConfig) {
	t.Helper()

	if s.unsupported(t, "bulk", config.Bulk.Supported) {
		body := map[string]any{"schemas": []core.SchemaURI{protocol.SchemaBulkRequest}, "Operations": []any{}}
		expectStatus(t, s.client.Post(t, "/Bulk", body), http.StatusNotImplemented, "RFC 7644 3.12")
		return
	}
	s.todo(t, "bulk checks are not written yet")
}

// RFC 7644 Table 8: 501 when the service provider does not support the requested operation.
func (s *Suite) unsupported(t *testing.T, feature string, supported bool) bool {
	t.Helper()

	if !s.strict {
		supports(t, feature, supported)
	}
	return !supported
}

func (s *Suite) todo(t *testing.T, format string, args ...any) {
	t.Helper()

	if s.strict {
		t.Fatalf("todo: "+format, args...)
	}
	t.Skipf("todo: "+format, args...)
}

func (s *Suite) should(t *testing.T, rule string) {
	t.Helper()

	if !s.strict {
		t.Skip("SHOULD: " + rule)
	}
}

func list[T any](t *testing.T, c *Client, path string) protocol.ListResponse[T] {
	t.Helper()

	res := c.Get(t, path)
	expectStatus(t, res, http.StatusOK, "RFC 7644 3.4.2")
	body := Decode[protocol.ListResponse[T]](t, res)
	if !slices.Contains(body.Schemas, protocol.SchemaListResponse) {
		t.Fatalf("scimtest: GET %s (RFC 7644 3.4.2): schemas %v, want %s", path, body.Schemas, protocol.SchemaListResponse)
	}
	return body
}

func expectStatus(t TB, res *http.Response, want int, section string) {
	t.Helper()

	if res.StatusCode != want {
		t.Fatalf("scimtest: %s %s (%s): status %d, want %d", res.Request.Method, res.Request.URL.RequestURI(), section, res.StatusCode, want)
	}
}
