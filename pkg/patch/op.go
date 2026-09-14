package patch

// Op is the kind of modification in a PATCH operation, per RFC 7644, Section 3.5.2.
type Op string

const (
	OpAdd     Op = "add"
	OpRemove  Op = "remove"
	OpReplace Op = "replace"
)
