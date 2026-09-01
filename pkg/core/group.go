package core

import "time"

// Member is one member reference of a Group, per RFC 7643, Section 4.2.
type Member struct {
	Value   string           `json:"value,omitempty"`
	Ref     string           `json:"$ref,omitempty"`
	Type    ResourceTypeName `json:"type,omitempty"`
	Display string           `json:"display,omitempty"`
}

// Group is the core Group resource defined in RFC 7643, Section 4.2.
type Group struct {
	Schemas     []SchemaURI `json:"schemas"`
	ID          string      `json:"id"`
	ExternalID  string      `json:"externalId,omitempty"`
	DisplayName string      `json:"displayName"`
	Members     []Member    `json:"members,omitempty"`
	Meta        Meta        `json:"meta"`
}

func (g *Group) ResourceID() string {
	return g.ID
}

func (g *Group) Location() string {
	return g.Meta.Location
}

func (g *Group) Timestamps() (created, updated time.Time) {
	return g.Meta.Created, g.Meta.LastModified
}
