package scimtest

import (
	"cmp"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
)

type discovering struct {
	suite     *Suite
	found     *discovery
	resources []*resource
}

func (d discovering) run(t *testing.T) {
	t.Helper()

	t.Run("advertises the ServiceProviderConfig schema", d.checkConfig)
	t.Run("lists the schemas of every resource type", d.checkSchemas)
	t.Run("retrieves each resource type and schema by id", d.checkByID)
	t.Run("returns 404 for an unknown resource type or schema", d.checkUnknownID)
	t.Run("rejects a filter on /Schemas and /ResourceTypes", d.checkFilter)
	t.Run("returns a SCIM 404 for an unknown endpoint", d.checkUnknownEndpoint)
	t.Run("advertises the RFC 7643 attribute characteristics", d.checkCharacteristics)
	t.Run("rejects a request without credentials", d.checkUnauthenticated)
	t.Run("rejects an invalid bearer token", d.checkInvalidToken)
	t.Run("rejects a write with a SCIM 405 that lists Allow", d.checkMethodNotAllowed)
}

func (d discovering) checkConfig(t *testing.T) {
	if !slices.Contains(d.found.config.Schemas, core.SchemaServiceProviderConfig) {
		t.Errorf("scimtest: GET /ServiceProviderConfig (RFC 7643 5): schemas %v, want %s", d.found.config.Schemas, core.SchemaServiceProviderConfig)
	}
}

func (d discovering) checkSchemas(t *testing.T) {
	for _, kind := range d.found.kinds {
		for _, uri := range schemaURIsOf(kind) {
			if d.found.schemas.Lookup(uri) == nil {
				t.Errorf("scimtest: GET /Schemas (RFC 7644 4): %s of resource type %s is missing", uri, kind.Name)
			}
		}
	}
}

// RFC 7644 Section 4: a single ResourceType or Schema is retrieved like a single resource.
func (d discovering) checkByID(t *testing.T) {
	for _, kind := range d.found.kinds {
		id := string(kind.ID)
		if id == "" {
			id = string(kind.Name)
		}
		got := d.get(t, "/ResourceTypes/"+url.PathEscape(id))
		if got["name"] != string(kind.Name) {
			t.Errorf("scimtest: GET /ResourceTypes/%s (RFC 7644 4): name %v, want %s", id, got["name"], kind.Name)
		}
	}
	for _, schema := range d.found.schemas {
		if got := d.get(t, "/Schemas/"+string(schema.ID)); got["id"] != string(schema.ID) {
			t.Errorf("scimtest: GET /Schemas/%s (RFC 7644 4): id %v", schema.ID, got["id"])
		}
	}
}

func (d discovering) checkUnknownID(t *testing.T) {
	for _, path := range []string{"/ResourceTypes/scimtest-" + newUnique(), "/Schemas/urn:scimtest:" + newUnique()} {
		RequireError(t, d.suite.client.Get(t, path), http.StatusNotFound, "")
	}
}

// RFC 7644 Section 4: a filter on these endpoints SHOULD be answered with 403.
func (d discovering) checkFilter(t *testing.T) {
	d.suite.should(t, "RFC 7644 4 403 for a filter on /Schemas or /ResourceTypes")
	query := "?" + filterBy(`name eq "User"`).Encode()

	for _, path := range []string{"/Schemas", "/ResourceTypes"} {
		RequireError(t, d.suite.client.Get(t, path+query), http.StatusForbidden, "")
	}
}

// RFC 7644 Section 3.12, Table 8: 404 for an endpoint that does not exist.
func (d discovering) checkUnknownEndpoint(t *testing.T) {
	RequireError(t, d.suite.client.Get(t, "/scimtest-"+newUnique()), http.StatusNotFound, "")
}

func (d discovering) checkCharacteristics(t *testing.T) {
	d.suite.should(t, "RFC 7643 8.7.1 attributes keep the characteristics of the core schemas")
	var golden []*core.Schema
	if err := json.Unmarshal(Golden(t, RFC7643ResourceSchemas), &golden); err != nil {
		t.Fatalf("scimtest: decoding %s: %v", RFC7643ResourceSchemas, err)
	}
	for _, want := range golden {
		if got := d.found.schemas.Lookup(want.ID); got != nil {
			compareAttributes(t, string(want.ID), want.Attributes, got.Attributes)
		}
	}
}

// RFC 7644 Section 2: the provider SHALL indicate its schemes via "WWW-Authenticate". RFC 9110 Section 15.5.2: a 401 MUST send it.
func (d discovering) checkUnauthenticated(t *testing.T) {
	d.expectChallenged(t, d.suite.client.anonymous())
}

// RFC 7644 Section 3.12, Table 8: 401 when the authorization header is invalid or missing.
func (d discovering) checkInvalidToken(t *testing.T) {
	d.expectChallenged(t, d.suite.client.anonymous().With(WithBearerToken("scimtest-invalid-"+newUnique())))
}

// RFC 9110 Section 15.5.6: a 405 MUST send an Allow header. RFC 7643 Sections 5 and 7 make ServiceProviderConfig and Schemas readOnly.
func (d discovering) checkMethodNotAllowed(t *testing.T) {
	for _, path := range []string{"/ServiceProviderConfig", "/ResourceTypes", "/Schemas"} {
		for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
			t.Run(method+" "+path, func(t *testing.T) { d.expectNotAllowed(t, method, path) })
		}
	}
}

func (d discovering) expectNotAllowed(t *testing.T, method, path string) {
	t.Helper()

	res := d.suite.client.Send(t, method, path, nil)
	if res.StatusCode == http.StatusNotImplemented || res.StatusCode == http.StatusNotFound {
		return
	}
	RequireError(t, res, http.StatusMethodNotAllowed, "")
	allow := res.Header.Get("Allow")
	if allow == "" || slices.Contains(strings.Split(strings.ReplaceAll(allow, " ", ""), ","), method) {
		t.Errorf("scimtest: %s %s (RFC 9110 15.5.6): Allow %q, want the methods it accepts without %s", method, path, allow, method)
	}
}

func (d discovering) expectChallenged(t *testing.T, c *Client) {
	t.Helper()

	if len(d.found.config.AuthenticationSchemes) == 0 || len(d.resources) == 0 {
		t.Skip("n/a: no authenticationSchemes or no resource types are advertised")
	}
	res := c.Get(t, d.resources[0].kind.Endpoint)

	RequireError(t, res, http.StatusUnauthorized, "")
	if res.Header.Get("WWW-Authenticate") == "" {
		t.Errorf("scimtest: GET %s (RFC 7644 2, RFC 9110 15.5.2): no WWW-Authenticate header", d.resources[0].kind.Endpoint)
	}
}

func (d discovering) get(t *testing.T, path string) map[string]any {
	t.Helper()
	return get[map[string]any](t, d.suite.client, path, "RFC 7644 4")
}

// RFC 7643 Section 2.2: an unstated characteristic takes its default.
func compareAttributes(t *testing.T, where string, want, got core.Attributes) {
	t.Helper()

	for _, expected := range want {
		path := where + ":" + expected.Name
		actual := got.Lookup(expected.Name)
		if actual == nil {
			t.Errorf("SHOULD: GET /Schemas (RFC 7643 7 and 8.7.1, a non-normative example): %s is missing", path)
			continue
		}
		for _, field := range []struct {
			name      string
			want, got any
		}{
			{"type", cmp.Or(expected.Type, core.TypeString), cmp.Or(actual.Type, core.TypeString)},
			{"multiValued", expected.MultiValued, actual.MultiValued},
			{"mutability", cmp.Or(expected.Mutability, core.MutabilityReadWrite), cmp.Or(actual.Mutability, core.MutabilityReadWrite)},
			{"returned", cmp.Or(expected.Returned, core.ReturnedDefault), cmp.Or(actual.Returned, core.ReturnedDefault)},
			{"uniqueness", cmp.Or(expected.Uniqueness, core.UniquenessNone), cmp.Or(actual.Uniqueness, core.UniquenessNone)},
			{"caseExact", expected.CaseExact, actual.CaseExact},
		} {
			if field.want != field.got {
				t.Errorf("SHOULD: GET /Schemas (RFC 7643 7 and 8.7.1, a non-normative example): %s %s is %v, want %v", path, field.name, field.got, field.want)
			}
		}
		compareAttributes(t, path, expected.SubAttributes, actual.SubAttributes)
	}
}
