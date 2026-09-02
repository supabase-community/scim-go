package core

// Attributes every SCIM resource shares, per RFC 7643, Section 3.1.
type CommonAttributes struct {
	Schemas []SchemaURI `json:"schemas"`
	ID      string      `json:"id"`
	Meta    Meta        `json:"meta"`
}
