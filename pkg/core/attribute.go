package core

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"time"
)

// Attribute describes one attribute of a schema, per RFC 7643, Section 7.
type Attribute struct {
	Name            string          `json:"name"`
	Type            AttributeType   `json:"type"`
	MultiValued     bool            `json:"multiValued"`
	Description     string          `json:"description"`
	Required        bool            `json:"required"`
	CanonicalValues []string        `json:"canonicalValues,omitempty"`
	CaseExact       bool            `json:"caseExact"`
	Mutability      Mutability      `json:"mutability"`
	Returned        Returned        `json:"returned"`
	Uniqueness      Uniqueness      `json:"uniqueness"`
	ReferenceTypes  []ReferenceType `json:"referenceTypes,omitempty"`
	SubAttributes   Attributes      `json:"subAttributes,omitempty"`
}

func NewAttribute(name string, attributeType AttributeType, description string) *Attribute {
	return &Attribute{
		Name:        name,
		Type:        attributeType,
		Description: description,
		Mutability:  MutabilityReadWrite,
		Returned:    ReturnedDefault,
		Uniqueness:  UniquenessNone,
	}
}

func (a *Attribute) AsRequired() *Attribute {
	a.Required = true
	return a
}

func (a *Attribute) AsMultiValued() *Attribute {
	a.MultiValued = true
	return a
}

func (a *Attribute) AsCaseExact() *Attribute {
	a.CaseExact = true
	return a
}

// AsImmutable sets "mutability" to "immutable", per RFC 7643, Section 7.
func (a *Attribute) AsImmutable() *Attribute {
	a.Mutability = MutabilityImmutable
	return a
}

// AsReadOnly sets "mutability" to "readOnly", per RFC 7643, Section 7.
func (a *Attribute) AsReadOnly() *Attribute {
	a.Mutability = MutabilityReadOnly
	return a
}

// AsWriteOnly sets "mutability" to "writeOnly", per RFC 7643, Section 7.
func (a *Attribute) AsWriteOnly() *Attribute {
	a.Mutability = MutabilityWriteOnly
	return a
}

// ReturnedAs sets "returned", per RFC 7643, Section 7.
func (a *Attribute) ReturnedAs(returned Returned) *Attribute {
	a.Returned = returned
	return a
}

// Suggesting sets "canonicalValues", per RFC 7643, Section 7.
func (a *Attribute) Suggesting(values ...string) *Attribute {
	a.CanonicalValues = values
	return a
}

// Referencing sets "referenceTypes", per RFC 7643, Section 7.
func (a *Attribute) Referencing(referenceTypes ...ReferenceType) *Attribute {
	a.ReferenceTypes = referenceTypes
	return a
}

func (a *Attribute) UniqueOn(uniqueness Uniqueness) *Attribute {
	a.Uniqueness = uniqueness
	return a
}

func (a *Attribute) With(subAttributes ...*Attribute) *Attribute {
	a.SubAttributes = subAttributes
	return a
}

func (a *Attribute) SubAttribute(name string) *Attribute {
	return a.SubAttributes.Lookup(name)
}

func (a *Attribute) Coerce(value any) (any, bool) {
	if value == nil {
		return nil, true
	}
	return coerce(a.Type, value)
}

func coerce(attributeType AttributeType, value any) (any, bool) {
	switch attributeType {
	case TypeString, TypeReference:
		s, ok := value.(string)
		return s, ok
	case TypeBinary:
		s, ok := value.(string)
		if !ok {
			return nil, false
		}
		// RFC 7643 Section 2.3.6: binary values are base64 encoded.
		if _, err := base64.StdEncoding.DecodeString(s); err != nil {
			return nil, false
		}
		return s, true
	case TypeBoolean:
		b, ok := value.(bool)
		return b, ok
	case TypeDecimal:
		switch n := value.(type) {
		case json.Number:
			f, err := n.Float64()
			return f, err == nil
		case float64:
			return n, true
		}
		return nil, false
	case TypeInteger:
		switch n := value.(type) {
		case json.Number:
			i, err := n.Int64()
			return i, err == nil
		case int64:
			return n, true
		case float64:
			return int64(n), n == math.Trunc(n)
		}
		return nil, false
	case TypeDateTime:
		s, ok := value.(string)
		if !ok {
			return nil, false
		}
		t, err := time.Parse(time.RFC3339, s)
		return t, err == nil
	}
	return nil, false
}
