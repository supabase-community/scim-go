package scimtest

import (
	"maps"
	"net/url"
	"slices"
	"strconv"
	"testing"
)

type paging struct {
	*resource
}

func (r paging) run(t *testing.T) {
	t.Helper()

	r.create(t, newUnique())
	r.create(t, newUnique())

	t.Run("returns only totalResults for count 0", func(t *testing.T) { r.expectPage(t, url.Values{countParam: {"0"}}, 0, 1) })
	t.Run("treats a negative count as 0", func(t *testing.T) { r.expectPage(t, url.Values{countParam: {"-1"}}, 0, 1) })
	t.Run("returns no more than count", func(t *testing.T) { r.expectPage(t, url.Values{countParam: {"1"}}, 1, 2) })
	t.Run("treats a startIndex below 1 as 1", func(t *testing.T) {
		query := url.Values{"startIndex": {"0"}, countParam: {"1"}}
		if page := r.list(t, query); page.StartIndex != 1 {
			t.Errorf("scimtest: %s (RFC 7644 3.4.2.4): startIndex %d, want 1", r.describe(query), page.StartIndex)
		}
	})
	t.Run("walks every result once, page by page", r.checkPageWalk)
	t.Run("returns no more than filter.maxResults", r.checkPageCap)
}

func (r paging) checkPageWalk(t *testing.T) {
	supports(t, "sort", r.config.Sort.Supported)
	supports(t, "filter", r.config.Filter.Supported)
	ids := r.createIDs(t, 3)
	query := byID(ids...)
	query.Set("sortBy", "id")
	query.Set(countParam, "3")
	want := idsOf(r.list(t, query).Resources)

	walked := make([]string, 0, len(ids))
	for start := range ids {
		page := maps.Clone(query)
		page.Set("startIndex", strconv.Itoa(start+1))
		page.Set(countParam, "1")
		walked = append(walked, idsOf(r.list(t, page).Resources)...)
	}
	if !slices.Equal(walked, want) {
		t.Errorf("scimtest: %s (RFC 7644 3.4.2.4): pages of 1 returned %v, want %v", r.describe(query), walked, want)
	}
}

func (r paging) checkPageCap(t *testing.T) {
	if r.transport.search {
		t.Skip("n/a: checked once, over GET")
	}
	most := r.config.Filter.MaxResults
	if most <= 0 || most > 200 {
		t.Skipf("n/a: filter.maxResults is %d", most)
	}
	for total := r.list(t, url.Values{countParam: {"0"}}).TotalResults; total <= most; total++ {
		r.create(t, newUnique())
	}
	query := url.Values{countParam: {strconv.Itoa(most + 1)}}

	if page := r.list(t, query); len(page.Resources) > most {
		t.Errorf("scimtest: %s (RFC 7643 5): %d resources, want at most filter.maxResults %d", r.describe(query), len(page.Resources), most)
	}
}
