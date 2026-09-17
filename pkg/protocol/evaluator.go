package protocol

import (
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
)

type Evaluator[Output any] interface {
	Compare(attribute *Attribute, op filter.Operator, value any) (Output, error)
	Present(attribute *Attribute) (Output, error)
	And(left, right Output) (Output, error)
	Or(left, right Output) (Output, error)
	Not(operand Output) (Output, error)
	ValuePath(attribute *core.Attribute, valueFilter func() (Output, error)) (Output, error)
}
