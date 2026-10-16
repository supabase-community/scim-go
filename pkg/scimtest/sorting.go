package scimtest

import (
	"net/http"
	"net/url"
	"testing"
)

type sorting struct {
	*resource
}

func (r sorting) run(t *testing.T) {
	t.Helper()

	t.Run("accepts sortBy", r.checkSortBy)
	t.Run("sorts ascending by default and descending on request", r.checkSortOrder)
}

func (r sorting) checkSortBy(t *testing.T) {
	query := url.Values{"sortBy": {"id"}}
	if r.suite.unsupported(t, "sort", r.config.Sort.Supported) {
		expectStatus(t, r.send(t, query), http.StatusNotImplemented, "RFC 7644 3.12")
		return
	}
	r.list(t, query)
}

func (r sorting) checkSortOrder(t *testing.T) {
	supports(t, "sort", r.config.Sort.Supported)
	supports(t, "filter", r.config.Filter.Supported)
	attribute := r.attribute(t, "readWrite singular string attribute", patchable)
	unique := newUnique()
	ids := make([]string, 0, 2)
	for _, prefix := range []string{"scimtest a ", "scimtest b "} {
		body := r.body(t, newUnique())
		body.Set(attribute.Name, prefix+unique)
		_, created := r.createBody(t, body)
		ids = append(ids, idOf(created))
	}
	query := byID(ids...)
	query.Set("sortBy", attribute.Name)

	r.expectIDs(t, query, ids, "RFC 7644 3.4.2.3")
	query.Set("sortOrder", "ascending")
	r.expectIDs(t, query, ids, "RFC 7644 3.4.2.3")
	query.Set("sortOrder", "descending")
	r.expectIDs(t, query, []string{ids[1], ids[0]}, "RFC 7644 3.4.2.3")
}
