package filter

// Operator is a SCIM comparison operator, per RFC 7644, Section 3.4.2.2.
type Operator string

const (
	OpEquals            Operator = "eq"
	OpNotEquals         Operator = "ne"
	OpContains          Operator = "co"
	OpStartsWith        Operator = "sw"
	OpEndsWith          Operator = "ew"
	OpGreaterThan       Operator = "gt"
	OpGreaterThanEquals Operator = "ge"
	OpLessThan          Operator = "lt"
	OpLessThanEquals    Operator = "le"
)
