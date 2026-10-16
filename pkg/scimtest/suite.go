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
	name     string
	check    func(r *resource, t *testing.T)
	listings bool
}

var sections = []section{
	{"3.3 Creating Resources", (*resource).checkCreate, false},
	{"3.4.1 Retrieving a Known Resource", (*resource).checkRead, false},
	{"3.4.2 Query Resources", (*resource).checkList, true},
	{"3.4.2.2 Filtering", (*resource).checkFilter, true},
	{"3.4.2.3 Sorting", (*resource).checkSort, true},
	{"3.4.2.4 Pagination", (*resource).checkPagination, true},
	{"3.4.2.5 Attributes", (*resource).checkAttributes, false},
	{"3.5.1 Replacing with PUT", (*resource).checkReplace, false},
	{"3.5.2 Modifying with PATCH", (*resource).checkPatch, false},
	{"3.6 Deleting Resources", (*resource).checkDelete, false},
}

var versionSection = section{"3.14 Versioning Resources", (*resource).checkVersioning, false}

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
	t.Run("RFC7643", func(t *testing.T) {
		t.Run("2.3.7 Reference", func(t *testing.T) {
			for _, reference := range newReferencing(resources) {
				t.Run(string(reference.source.kind.Name)+"."+reference.attribute.Name+" to "+string(reference.target.kind.Name), reference.run)
			}
		})
		t.Run("3.1 Common Attributes", identifying{resources}.run)
	})
	t.Run("RFC7644", func(t *testing.T) {
		for _, sec := range sections {
			s.section(t, resources, sec)
		}
		t.Run("3.7 Bulk Operations", func(t *testing.T) { s.checkBulk(t, &d.config) })
		s.section(t, resources, versionSection)
		t.Run("4 Service Provider Configuration Endpoints", discovering{s, d, resources}.run)
	})
}

func (s *Suite) section(t *testing.T, resources []*resource, sec section) {
	t.Helper()

	t.Run(sec.name, func(t *testing.T) {
		for _, r := range resources {
			t.Run(string(r.kind.Name), func(t *testing.T) { s.run(t, r, sec) })
		}
	})
}

func (s *Suite) run(t *testing.T, r *resource, sec section) {
	t.Helper()

	if !sec.listings {
		sec.check(r, t)
		return
	}
	for _, tr := range transports {
		t.Run(tr.name, func(t *testing.T) { sec.check(r.over(t, tr), t) })
	}
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
		if schemas := d.schemasOf(kind); schemas != nil {
			resources = append(resources, &resource{suite: s, config: &d.config, kind: kind, schemas: schemas, fixture: s.fixtures[kind.Name]})
		}
	}
	return resources
}

func (d *discovery) schemasOf(kind core.ResourceType) core.Schemas {
	if d.schemas.Lookup(kind.Schema) == nil {
		return nil
	}
	schemas := core.Schemas{}
	for _, uri := range schemaURIsOf(kind) {
		if schema := d.schemas.Lookup(uri); schema != nil {
			schemas = append(schemas, schema)
		}
	}
	return schemas
}

func schemaURIsOf(kind core.ResourceType) []core.SchemaURI {
	uris := make([]core.SchemaURI, 0, 1+len(kind.SchemaExtensions))
	uris = append(uris, kind.Schema)
	for _, extension := range kind.SchemaExtensions {
		uris = append(uris, extension.Schema)
	}
	return uris
}

func (s *Suite) checkBulk(t *testing.T, config *core.ServiceProviderConfig) {
	t.Helper()

	if s.unsupported(t, "bulk", config.Bulk.Supported) {
		body := map[string]any{schemasAttr: []core.SchemaURI{protocol.SchemaBulkRequest}, "Operations": []any{}}
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
