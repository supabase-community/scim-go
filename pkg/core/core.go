// Package core implements the SCIM 2.0 core schema defined in RFC 7643.
package core

// SchemaURI identifies a SCIM schema, per RFC 7643, Section 3.
type SchemaURI string

// ResourceTypeName names a resource type, per RFC 7643, Section 6.
type ResourceTypeName string

func (n ResourceTypeName) Reference() ReferenceType { return ReferenceType(n) }

// ReferenceType is a "referenceTypes" value, per RFC 7643, Section 7.
type ReferenceType string

// AttributeType is the data type of an attribute, per RFC 7643, Section 7.
type AttributeType string

const (
	TypeString    AttributeType = "string"
	TypeBoolean   AttributeType = "boolean"
	TypeDecimal   AttributeType = "decimal"
	TypeInteger   AttributeType = "integer"
	TypeDateTime  AttributeType = "dateTime"
	TypeReference AttributeType = "reference"
	TypeComplex   AttributeType = "complex"
)

// Mutability states when an attribute may be (re)defined, per RFC 7643, Section 7.
type Mutability string

const (
	MutabilityReadOnly  Mutability = "readOnly"
	MutabilityReadWrite Mutability = "readWrite"
	MutabilityImmutable Mutability = "immutable"
	MutabilityWriteOnly Mutability = "writeOnly"
)

// Returned states when an attribute is included in a response, per RFC 7643, Section 7.
type Returned string

const (
	ReturnedAlways  Returned = "always"
	ReturnedNever   Returned = "never"
	ReturnedDefault Returned = "default"
	ReturnedRequest Returned = "request"
)

// Uniqueness states how the service provider enforces uniqueness, per RFC 7643, Section 7.
type Uniqueness string

const (
	UniquenessNone   Uniqueness = "none"
	UniquenessServer Uniqueness = "server"
	UniquenessGlobal Uniqueness = "global"
)

// The reference types of RFC 7643, Section 7 that are not resource types.
const (
	ReferenceExternal ReferenceType = "external"
	ReferenceURI      ReferenceType = "uri"
)

type SupportedFeature struct {
	Supported bool `json:"supported"`
}

type BulkFeature struct {
	Supported      bool `json:"supported"`
	MaxOperations  int  `json:"maxOperations"`
	MaxPayloadSize int  `json:"maxPayloadSize"`
}

type FilterFeature struct {
	Supported  bool `json:"supported"`
	MaxResults int  `json:"maxResults"`
}
