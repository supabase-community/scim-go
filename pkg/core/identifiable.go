package core

// Identifiable lets generic infrastructure read and assign a resource's ID and metadata.
type Identifiable interface {
	Resource
	SetID(id string)
	GetMeta() Meta
	SetMeta(meta Meta)
}
