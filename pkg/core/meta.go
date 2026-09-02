package core

import "time"

// Meta is the resource metadata common attribute defined in RFC 7643, Section 3.1.
type Meta struct {
	ResourceType ResourceTypeName `json:"resourceType"`
	Created      time.Time        `json:"created,omitzero"`
	LastModified time.Time        `json:"lastModified,omitzero"`
	Location     string           `json:"location,omitempty"`
	Version      string           `json:"version,omitempty"`
}

// TODO:: Remove this
func NewMeta(baseURL string, kind Kind) Meta {
	return Meta{
		ResourceType: kind.Name,
		Location:     kind.Location(baseURL),
	}
}

// TODO:: Remove this
func (m Meta) For(resource Resource) Meta {
	m.Location = Join(m.Location, resource.ResourceID())
	return m
}
