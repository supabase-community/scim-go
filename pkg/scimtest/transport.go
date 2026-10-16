package scimtest

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

const (
	countParam      = "count"
	attributesParam = "attributes"
	excludedParam   = "excludedAttributes"
)

var transports = []transport{{name: "GET"}, {name: "POST .search", search: true}}

type transport struct {
	name   string
	search bool
}

func (tr transport) send(t *testing.T, c *Client, endpoint string, query url.Values) *http.Response {
	t.Helper()

	if tr.search {
		return c.Post(t, endpoint+"/.search", searchBody(query))
	}
	return c.Get(t, endpoint+"?"+query.Encode())
}

func (tr transport) describe(endpoint string, query url.Values) string {
	if tr.search {
		return strings.TrimSpace("POST " + endpoint + "/.search " + query.Encode())
	}
	return strings.TrimSuffix("GET "+endpoint+"?"+query.Encode(), "?")
}

// RFC 7644 Section 3.4.3: a search request carries the query parameters as JSON, with attribute lists as arrays.
func searchBody(query url.Values) map[string]any {
	body := map[string]any{schemasAttr: []core.SchemaURI{protocol.SchemaSearchRequest}}
	for key, values := range query {
		switch key {
		case "startIndex", countParam:
			number, _ := strconv.Atoi(values[0])
			body[key] = number
		case attributesParam, excludedParam:
			body[key] = strings.Split(values[0], ",")
		default:
			body[key] = values[0]
		}
	}
	return body
}
