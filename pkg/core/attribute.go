package core

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
	SubAttributes   []*Attribute    `json:"subAttributes,omitempty"`
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
