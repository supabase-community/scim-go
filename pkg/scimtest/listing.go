package scimtest

import (
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type listing struct {
	*resource
}

func (r listing) run(t *testing.T) {
	t.Helper()

	t.Run("returns a ListResponse", func(t *testing.T) { r.list(t, url.Values{}) })
	t.Run("returns totalResults 0 when nothing matches", r.checkListEmpty)
	t.Run("locates every listed resource", r.checkListLocations)
	t.Run("rejects a search without the SearchRequest schema", r.checkSearchSchema)
}

func (r listing) checkListEmpty(t *testing.T) {
	supports(t, "filter", r.config.Filter.Supported)
	query := byID("scimtest-" + newUnique())

	res := r.send(t, query)
	if page := r.page(t, query, res); page.TotalResults != 0 || len(page.Resources) != 0 {
		t.Errorf("scimtest: %s (RFC 7644 3.4.2): totalResults %d with %d resources, want 0", r.describe(query), page.TotalResults, len(page.Resources))
	}
	if _, ok := Decode[map[string]any](t, res)["totalResults"]; !ok {
		t.Errorf("scimtest: %s (RFC 7644 3.4.2): totalResults is missing", r.describe(query))
	}
}

func (r listing) checkListLocations(t *testing.T) {
	r.create(t, newUnique())
	query := url.Values{}

	for _, object := range r.list(t, query).Resources {
		if location, ok := metaOf(object)["location"].(string); ok && !strings.HasSuffix(location, r.path(object)) {
			t.Errorf("scimtest: %s (RFC 7643 3.1): meta.location %q, want the URI of %s", r.describe(query), location, r.path(object))
		}
	}
}

// RFC 7644 Section 3.4.3: query requests MUST be identified with the SearchRequest URI.
func (r listing) checkSearchSchema(t *testing.T) {
	if !r.transport.search {
		t.Skip("n/a: GET has no request body")
	}
	r.suite.should(t, "RFC 7644 Table 9 400 invalidSyntax for a search without the SearchRequest schema")
	body := searchBody(url.Values{countParam: {"0"}})
	delete(body, schemasAttr)

	RequireError(t, r.client().Post(t, r.kind.Endpoint+"/.search", body), http.StatusBadRequest, scimerrors.InvalidSyntax)
}
