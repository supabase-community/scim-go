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
