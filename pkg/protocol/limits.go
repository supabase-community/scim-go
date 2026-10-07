package protocol

import (
	"encoding/json"
	"io"
	"net/url"
	"strconv"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// Limits holds the count bounds of RFC 7644 Section 3.4.2.4 and the MaxOperations cap that zero lifts.
type Limits struct {
	DefaultCount  int
	MaxCount      int
	MaxOperations int
}

var DefaultLimits = Limits{DefaultCount: 100, MaxCount: 100, MaxOperations: 100}

// ParseSearchRequest reads the query parameters of RFC 7644, Section 3.4.2.
func (l Limits) ParseSearchRequest(values url.Values) (*SearchRequest, error) {
	startIndex, err := intParam(values, "startIndex", 1)
	if err != nil {
		return nil, err
	}

	count, err := intParam(values, "count", l.DefaultCount)
	if err != nil {
		return nil, err
	}

	return l.normalize(&SearchRequest{
		Schemas:            []core.SchemaURI{SchemaSearchRequest},
		Attributes:         listParam(values, "attributes"),
		ExcludedAttributes: listParam(values, "excludedAttributes"),
		Filter:             values.Get("filter"),
		SortBy:             values.Get("sortBy"),
		SortOrder:          SortOrder(values.Get("sortOrder")),
		StartIndex:         startIndex,
		Count:              count,
	})
}

// DecodeSearchRequest reads a POST ".search" body; RFC 7644 Section 3.4.3: it MUST contain the SearchRequest "schemas" URI.
func (l Limits) DecodeSearchRequest(body io.Reader) (*SearchRequest, error) {
	raw, err := read(body)
	if err != nil {
		return nil, err
	}
	if _, err := objectOf(raw); err != nil {
		return nil, err
	}
	req := &SearchRequest{Count: l.DefaultCount, inBody: true}
	if err := json.Unmarshal(raw, req); err != nil {
		return nil, mistyped(err)
	}
	if err := requireSchema(req.Schemas, SchemaSearchRequest); err != nil {
		return nil, err
	}
	return l.normalize(req)
}

func (l Limits) DecodePatchRequest(body io.Reader) (*PatchRequest, error) {
	req, err := DecodePatchRequest(body)
	if err != nil {
		return nil, err
	}
	if l.MaxOperations > 0 && len(req.Operations) > l.MaxOperations {
		return nil, scimerrors.ErrTooLarge(`"Operations" must contain at most ` + strconv.Itoa(l.MaxOperations) + " operations")
	}
	return req, nil
}

func (l Limits) normalize(req *SearchRequest) (*SearchRequest, error) {
	switch req.SortOrder {
	case SortAscending, SortDescending:
	case "":
		if req.SortBy != "" {
			req.SortOrder = SortAscending
		}
	default:
		return nil, scimerrors.ErrInvalidValue(`"sortOrder" must be "ascending" or "descending"`)
	}
	req.StartIndex = max(req.StartIndex, 1)
	req.Count = min(max(req.Count, 0), l.MaxCount)
	return req, nil
}

func intParam(values url.Values, name string, fallback int) (int, error) {
	raw := values.Get(name)
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, scimerrors.ErrInvalidValue(strconv.Quote(name) + " must be an integer")
	}
	return value, nil
}
