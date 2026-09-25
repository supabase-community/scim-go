package protocol

import (
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
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

// Validate rejects a malformed filter or an unknown sortBy, per RFC 7644, Section 3.4.2.
func (s *SearchRequest) Validate(schemas core.Schemas) error {
	if s.Filter != "" {
		if _, err := filter.Parse(s.Filter); err != nil {
			return scimerrors.ErrInvalidFilter(err.Error())
		}
	}
	if s.SortBy != "" {
		_, _, err := s.SortAttribute(schemas)
		return err
	}
	return nil
}

// SortAttribute resolves sortBy to its attribute and that attribute's parent, per RFC 7644, Section 3.4.2.3.
func (s *SearchRequest) SortAttribute(schemas core.Schemas) (parent, attribute *core.Attribute, err error) {
	path, err := filter.NewAttrPath(s.SortBy)
	if err != nil {
		return nil, nil, scimerrors.ErrInvalidValue(err.Error())
	}
	parent, ok := schemas.Resolve(core.SchemaURI(path.URI), path.Name, "")
	if !ok {
		return nil, nil, scimerrors.ErrInvalidValue("Unknown sortBy")
	}
	attribute = parent
	if path.SubAttribute != "" {
		attribute = parent.SubAttribute(path.SubAttribute)
	}
	if attribute == nil {
		return nil, nil, scimerrors.ErrInvalidValue("Unknown sortBy")
	}
	if hidden(attribute) {
		return nil, nil, scimerrors.ErrInvalidValue(`"sortBy" must not name a writeOnly or returned "never" attribute`)
	}
	// RFC 7644 Section 3.4.2.3: a complex "sortBy" must be a path to a sub-attribute.
	if attribute.Type == core.TypeComplex {
		return nil, nil, scimerrors.ErrInvalidValue(`"sortBy" must name a sub-attribute of a complex attribute`)
	}
	return parent, attribute, nil
}

func (s *SearchRequest) Projection(schemas core.Schemas) (Projection, error) {
	return newProjection(schemas, s.Attributes, s.ExcludedAttributes)
}
