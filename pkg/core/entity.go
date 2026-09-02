package core

type Entity struct {
	Schemas []SchemaURI `json:"schemas"`
	ID      string      `json:"id"`
	Meta    Meta        `json:"meta"`
}
