package protocol_test

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

func TestParseSearchRequest(t *testing.T) {
	for _, tc := range []struct {
		name  string
		query string
		want  protocol.SearchRequest
	}{
		{
			name:  "defaults to the whole first page when the client asks for nothing",
			query: "",
			want:  protocol.SearchRequest{StartIndex: 1, Count: protocol.DefaultLimits.DefaultCount},
		},
		{
			name:  "honours the window the client asked for",
			query: "startIndex=11&count=10",
			want:  protocol.SearchRequest{StartIndex: 11, Count: 10},
		},
		{
			name:  "caps a count larger than the provider is willing to return",
			query: "count=5000",
			want:  protocol.SearchRequest{StartIndex: 1, Count: protocol.DefaultLimits.MaxCount},
		},
		{
			name:  "reads a start index below the first as the first",
			query: "startIndex=0",
			want:  protocol.SearchRequest{StartIndex: 1, Count: protocol.DefaultLimits.DefaultCount},
		},
		{
			name:  "reads a negative start index as the first",
			query: "startIndex=-7",
			want:  protocol.SearchRequest{StartIndex: 1, Count: protocol.DefaultLimits.DefaultCount},
		},
		{
			name:  "reads a negative count as none",
			query: "count=-5",
			want:  protocol.SearchRequest{StartIndex: 1, Count: 0},
		},
		{
			name:  "asks for the total alone with a count of none",
			query: "count=0",
			want:  protocol.SearchRequest{StartIndex: 1, Count: 0},
		},
		{
			name:  "leaves the order empty when attribute is empty",
			query: "sortBy=",
			want:  protocol.SearchRequest{StartIndex: 1, Count: protocol.DefaultLimits.DefaultCount, SortBy: "", SortOrder: ""},
		},
		{
			name:  "sorts ascending by default once an attribute is named",
			query: "sortBy=userName",
			want:  protocol.SearchRequest{StartIndex: 1, Count: protocol.DefaultLimits.DefaultCount, SortBy: "userName", SortOrder: protocol.SortAscending},
		},
		{
			name:  "sorts descending when asked",
			query: "sortBy=name.givenName&sortOrder=descending",
			want:  protocol.SearchRequest{StartIndex: 1, Count: protocol.DefaultLimits.DefaultCount, SortBy: "name.givenName", SortOrder: protocol.SortDescending},
		},
		{
			name:  "leaves the order unsaid when no attribute is named",
			query: "sortOrder=descending",
			want:  protocol.SearchRequest{StartIndex: 1, Count: protocol.DefaultLimits.DefaultCount, SortOrder: protocol.SortDescending},
		},
		{
			name:  "reads the attributes as the comma separated values they are",
			query: "attributes=userName,active",
			want: protocol.SearchRequest{
				StartIndex: 1,
				Count:      protocol.DefaultLimits.DefaultCount,
				Attributes: []string{"userName", "active"},
			},
		},
		{
			name:  "reads the excluded attributes as the comma separated values they are",
			query: "excludedAttributes=meta,groups",
			want: protocol.SearchRequest{
				StartIndex:         1,
				Count:              protocol.DefaultLimits.DefaultCount,
				ExcludedAttributes: []string{"meta", "groups"},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request, err := parseQuery(t, tc.query)

			require.NoError(t, err)

			tc.want.Schemas = []core.SchemaURI{protocol.SchemaSearchRequest}
			assert.Equal(t, &tc.want, request)
		})
	}

	for _, tc := range []struct {
		name, query, detail string
	}{
		{"a start index that is not a number", "startIndex=first", "startIndex"},
		{"a count that is not a number", "count=all", "count"},
		{"an order that is neither ascending nor descending", "sortBy=userName&sortOrder=sideways", "sortOrder"},
	} {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			request, err := parseQuery(t, tc.query)

			require.Nil(t, request)
			require.ErrorIs(t, err, scimerrors.ErrInvalidValue(""))
			assert.Contains(t, err.Error(), tc.detail)
		})
	}
}

func TestDecodeSearchRequest(t *testing.T) {
	const schemas = `"schemas":["urn:ietf:params:scim:api:messages:2.0:SearchRequest"]`

	for _, tc := range []struct {
		name string
		body string
		want protocol.SearchRequest
	}{
		{
			name: "decodes the body a client posts to .search",
			body: `{` + schemas + `,"attributes":["displayName","userName"],"filter":"displayName sw \"smith\"","startIndex":11,"count":10}`,
			want: protocol.SearchRequest{
				Attributes: []string{"displayName", "userName"},
				Filter:     `displayName sw "smith"`,
				StartIndex: 11,
				Count:      10,
			},
		},
		{
			name: "defaults to the whole first page when the client asks for nothing",
			body: `{` + schemas + `}`,
			want: protocol.SearchRequest{StartIndex: 1, Count: protocol.DefaultLimits.DefaultCount},
		},
		{
			name: "keeps a count of zero",
			body: `{` + schemas + `,"count":0}`,
			want: protocol.SearchRequest{StartIndex: 1},
		},
		{
			name: "caps a count larger than the provider is willing to return",
			body: `{` + schemas + `,"count":5000}`,
			want: protocol.SearchRequest{StartIndex: 1, Count: protocol.DefaultLimits.MaxCount},
		},
		{
			name: "sorts ascending when sortBy has no sortOrder",
			body: `{` + schemas + `,"sortBy":"userName"}`,
			want: protocol.SearchRequest{SortBy: "userName", SortOrder: protocol.SortAscending, StartIndex: 1, Count: protocol.DefaultLimits.DefaultCount},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request, err := protocol.DefaultLimits.DecodeSearchRequest(strings.NewReader(tc.body))

			require.NoError(t, err)

			tc.want.Schemas = []core.SchemaURI{protocol.SchemaSearchRequest}
			assert.EqualExportedValues(t, &tc.want, request)
		})
	}

	for _, tc := range []struct {
		name, body string
		want       error
	}{
		{"a body that is not an object", `[]`, scimerrors.ErrInvalidSyntax("")},
		{"a body without the SearchRequest schema", `{"filter":"userName pr"}`, scimerrors.ErrInvalidSyntax("")},
		{"a count that is not a number", `{` + schemas + `,"count":"all"}`, scimerrors.ErrInvalidSyntax("")},
		{"an order that is neither ascending nor descending", `{` + schemas + `,"sortBy":"userName","sortOrder":"sideways"}`, scimerrors.ErrInvalidValue("")},
	} {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			request, err := protocol.DefaultLimits.DecodeSearchRequest(strings.NewReader(tc.body))

			require.Nil(t, request)
			assert.ErrorIs(t, err, tc.want)
		})
	}
}

func TestSearchRequest(t *testing.T) {
	t.Run("converts the 1-based start index into a 0-based offset", func(t *testing.T) {
		assert.Equal(t, 0, (&protocol.SearchRequest{StartIndex: 1}).Offset())
		assert.Equal(t, 10, (&protocol.SearchRequest{StartIndex: 11}).Offset())
	})

	t.Run("reads a start index it was never given as the first", func(t *testing.T) {
		assert.Equal(t, 0, (&protocol.SearchRequest{}).Offset())
	})

	t.Run("reports the direction of the sort", func(t *testing.T) {
		assert.True(t, (&protocol.SearchRequest{SortOrder: protocol.SortDescending}).Descending())
		assert.False(t, (&protocol.SearchRequest{SortOrder: protocol.SortAscending}).Descending())
		assert.False(t, (&protocol.SearchRequest{}).Descending())
	})

	t.Run("builds a projection from its own attributes and excludedAttributes", func(t *testing.T) {
		schemas := []*core.Schema{(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(core.NewAttribute("userName", core.TypeString))}
		request := &protocol.SearchRequest{Attributes: []string{"userName"}}

		projection, err := request.Projection(schemas)
		require.NoError(t, err)

		raw, err := json.Marshal(projection.Of(map[string]any{"userName": "bjensen", "id": "1"}))
		require.NoError(t, err)
		assert.JSONEq(t, `{"schemas": ["`+string(core.SchemaUser)+`"], "userName": "bjensen", "id": "1"}`, string(raw))
	})

	t.Run("reports an invalid attribute name instead of panicking", func(t *testing.T) {
		schemas := []*core.Schema{(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(core.NewAttribute("userName", core.TypeString))}
		request := &protocol.SearchRequest{Attributes: []string{"1bad"}}

		_, err := request.Projection(schemas)

		require.ErrorIs(t, err, scimerrors.ErrInvalidValue(""))
	})

	// RFC 7644 Section 3.9: "attributes" and "excludedAttributes" are mutually exclusive.
	t.Run("rejects attributes together with excludedAttributes", func(t *testing.T) {
		schemas := []*core.Schema{(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(core.NewAttribute("userName", core.TypeString))}
		request := &protocol.SearchRequest{Attributes: []string{"userName"}, ExcludedAttributes: []string{"userName"}}

		_, err := request.Projection(schemas)

		require.ErrorIs(t, err, scimerrors.ErrInvalidValue(""))
		assert.Contains(t, err.Error(), "mutually exclusive")
	})

	t.Run("validates its filter and sortBy", func(t *testing.T) {
		schemas := []*core.Schema{(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("userName", core.TypeString),
			core.NewAttribute("name", core.TypeComplex).With(core.NewAttribute("givenName", core.TypeString)),
			core.NewMultiValuedAttribute("emails"),
		)}
		tt := []struct {
			request protocol.SearchRequest
			valid   bool
		}{
			{protocol.SearchRequest{}, true},
			{protocol.SearchRequest{Filter: `userName eq "bjensen"`, SortBy: "name.givenName"}, true},
			{protocol.SearchRequest{Filter: `userName eq`}, false},
			{protocol.SearchRequest{SortBy: "1bad"}, false},
			{protocol.SearchRequest{SortBy: "nickName"}, false},
			{protocol.SearchRequest{SortBy: "name.familyName"}, false},
			{protocol.SearchRequest{SortBy: "name"}, false},
			{protocol.SearchRequest{SortBy: "emails"}, true},
			{protocol.SearchRequest{SortBy: "emails.value"}, true},
		}
		for _, tc := range tt {
			err := tc.request.Validate(schemas)
			assert.Equal(t, tc.valid, err == nil, "%+v", tc.request)
		}
	})

	// RFC 7644 Section 7.5.2: a GET filter with sensitive information SHOULD be refused with 403.
	t.Run("refuses a query filter on a bare multi-valued writeOnly attribute with sensitive", func(t *testing.T) {
		schemas := []*core.Schema{(&core.Schema{ID: core.SchemaUser, Name: "User"}).With(
			core.NewAttribute("secrets", core.TypeComplex).AsMultiValued().AsWriteOnly().With(core.NewAttribute("value", core.TypeString)),
		)}
		for _, text := range []string{`secrets sw "a"`, `secrets co "a"`, `secrets gt "a"`} {
			err := (&protocol.SearchRequest{Filter: text}).Validate(schemas)
			require.ErrorIs(t, err, scimerrors.ErrSensitive(""), text)
		}
	})

	t.Run("serializes as the SearchRequest of RFC 7644, Section 3.4.3", func(t *testing.T) {
		request := &protocol.SearchRequest{
			Schemas:            []core.SchemaURI{protocol.SchemaSearchRequest},
			Attributes:         []string{"displayName", "userName"},
			ExcludedAttributes: []string{"meta"},
			Filter:             `displayName sw "smith"`,
			SortBy:             "displayName",
			SortOrder:          protocol.SortAscending,
			StartIndex:         1,
			Count:              10,
		}

		body, err := json.Marshal(request)

		require.NoError(t, err)
		require.JSONEq(t, `{
			"schemas": ["urn:ietf:params:scim:api:messages:2.0:SearchRequest"],
			"attributes": ["displayName", "userName"],
			"excludedAttributes": ["meta"],
			"filter": "displayName sw \"smith\"",
			"sortBy": "displayName",
			"sortOrder": "ascending",
			"startIndex": 1,
			"count": 10
		}`, string(body))
	})
}

func parseQuery(t *testing.T, query string) (*protocol.SearchRequest, error) {
	t.Helper()

	values, err := url.ParseQuery(query)
	require.NoError(t, err)

	return protocol.DefaultLimits.ParseSearchRequest(values)
}
