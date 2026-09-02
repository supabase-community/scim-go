package core

type Entity struct {
	Schemas []SchemaURI `json:"schemas"`
	ID      string      `json:"id"`
	// ExternalID string      `json:"externalId,omitempty"`
	Meta Meta `json:"meta"`
}
