package filter

// Operator is a SCIM attribute operator, per RFC 7644, Section 3.4.2.2, Table 3.
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
	OpPresent           Operator = "pr"
)

// The logical operators of RFC 7644, Section 3.4.2.2, distinct from the attribute operators of Table 3.
const (
	OpAnd Operator = "and"
	OpOr  Operator = "or"
)
