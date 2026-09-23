package protocol_test

import (
	"encoding/json"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

func parseQuery(t *testing.T, query string) (*protocol.SearchRequest, error) {
	t.Helper()

	values, err := url.ParseQuery(query)
	require.NoError(t, err)

	return protocol.DefaultLimits.ParseSearchRequest(values)
}

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
		{"attributes together with excluded attributes", "attributes=userName&excludedAttributes=meta", "mutually exclusive"},
	} {
		t.Run("rejects "+tc.name, func(t *testing.T) {
			request, err := parseQuery(t, tc.query)

			require.Nil(t, request)
			require.ErrorIs(t, err, scimerrors.ErrInvalidValue(""))
			assert.Contains(t, err.Error(), tc.detail)
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

		raw, err := json.Marshal(request.Projection(schemas).Of(map[string]any{"userName": "bjensen", "id": "1"}))

		require.NoError(t, err)
		assert.JSONEq(t, `{"userName": "bjensen", "id": "1"}`, string(raw))
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

	t.Run("decodes the body a client posts to .search", func(t *testing.T) {
		var request protocol.SearchRequest
		require.NoError(t, json.Unmarshal([]byte(`{
			"schemas": ["urn:ietf:params:scim:api:messages:2.0:SearchRequest"],
			"filter": "userName eq \"bjensen\"",
			"startIndex": 11,
			"count": 10
		}`), &request))

		assert.Equal(t, `userName eq "bjensen"`, request.Filter)
		assert.Equal(t, 10, request.Offset())
	})
}
