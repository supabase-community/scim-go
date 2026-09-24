package protocol

import (
	"github.com/supabase-community/scim-go/pkg/core"
)

// SortOrder is the direction a sort runs in, per RFC 7644, Section 3.4.2.3.
type SortOrder string

const (
	SortAscending  SortOrder = "ascending"
	SortDescending SortOrder = "descending"
)

// SearchRequest is the query of RFC 7644, Section 3.4.3.
type SearchRequest struct {
	Schemas            []core.SchemaURI `json:"schemas,omitempty"`
	Attributes         []string         `json:"attributes,omitempty"`
	ExcludedAttributes []string         `json:"excludedAttributes,omitempty"`
	Filter             string           `json:"filter,omitempty"`
	SortBy             string           `json:"sortBy,omitempty"`
	SortOrder          SortOrder        `json:"sortOrder,omitempty"`
	StartIndex         int              `json:"startIndex,omitempty"`
	Count              int              `json:"count,omitempty"`
}

// Offset is the zero-based start index, per Table 6 of RFC 7644, Section 3.4.2.4.
func (s *SearchRequest) Offset() int {
	if s.StartIndex < 1 {
		return 0
	}
	return s.StartIndex - 1
}

func (s *SearchRequest) Descending() bool {
	return s.SortOrder == SortDescending
}

func (s *SearchRequest) Projection(schemas core.Schemas) (Projection, error) {
	return newProjection(schemas, s.Attributes, s.ExcludedAttributes)
}
