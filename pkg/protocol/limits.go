package protocol

import (
	"io"
	"net/url"
	"strconv"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// Limits holds the count bounds of RFC 7644 Section 3.4.2.4, the MaxOperations, MaxFilterEvaluations, MaxWriteBytes, and MaxResourceBytes caps that zero lifts, and the PatchRetries of a versionless PATCH that loses a race, where zero means none.
type Limits struct {
	DefaultCount         int
	MaxCount             int
	MaxOperations        int
	MaxFilterEvaluations int
	MaxWriteBytes        int
	MaxResourceBytes     int
	PatchRetries         int
}

var DefaultLimits = Limits{DefaultCount: 100, MaxCount: 100, MaxOperations: 100, MaxFilterEvaluations: 10_000_000, MaxWriteBytes: 8 << 20, MaxResourceBytes: 10 << 20, PatchRetries: 2}

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

	sortOrder, err := sortOrderParam(values)
	if err != nil {
		return nil, err
	}

	attributes, excluded, err := parseAttributeParams(values)
	if err != nil {
		return nil, err
	}

	return &SearchRequest{
		Schemas:            []core.SchemaURI{SchemaSearchRequest},
		Attributes:         attributes,
		ExcludedAttributes: excluded,
		Filter:             values.Get("filter"),
		SortBy:             values.Get("sortBy"),
		SortOrder:          sortOrder,
		StartIndex:         max(startIndex, 1),
		Count:              min(max(count, 0), l.MaxCount),
	}, nil
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

func sortOrderParam(values url.Values) (SortOrder, error) {
	switch order := SortOrder(values.Get("sortOrder")); order {
	case SortAscending, SortDescending:
		return order, nil
	case "":
		if values.Get("sortBy") != "" {
			return SortAscending, nil
		}
		return "", nil
	default:
		return "", scimerrors.ErrInvalidValue(`"sortOrder" must be "ascending" or "descending"`)
	}
}
