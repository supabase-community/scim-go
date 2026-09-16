package core

// Base holds the attributes every SCIM resource shares, per RFC 7643, Section 3.1.
type Base struct {
	Schemas    []SchemaURI `json:"schemas"`
	ID         string      `json:"id"`
	ExternalID string      `json:"externalId,omitempty"`
	Meta       Meta        `json:"meta"`
}

func (b *Base) SetID(id string) {
	b.ID = id
}

func (b *Base) GetMeta() Meta {
	return b.Meta
}

func (b *Base) SetMeta(meta Meta) {
	b.Meta = meta
}
