package protocol

import "github.com/supabase-community/scim-go/pkg/filter"

type Evaluator[Output any] interface {
	Compare(attribute *Attribute, op filter.Operator, value any) (Output, error)
	Present(attribute *Attribute) (Output, error)
	And(left, right Output) (Output, error)
	Or(left, right Output) (Output, error)
	Not(operand Output) (Output, error)
	// RFC 7644 3.4.2.2 - valueFilter matches one element; attributes inside it carry Parent.
	ValuePath(attribute *Attribute, valueFilter func() (Output, error)) (Output, error)
}
