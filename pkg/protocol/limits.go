package protocol

import (
	"net/url"
	"strconv"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// Limits are the pagination bounds of one provider, per Table 6 of RFC 7644, Section 3.4.2.4.
type Limits struct {
	DefaultCount int
	MaxCount     int
}

var DefaultLimits = Limits{DefaultCount: 100, MaxCount: 100}

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
