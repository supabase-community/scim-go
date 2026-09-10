package protocol

import (
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
)

type Evaluator[T any] interface {
	Compare(attribute *core.Attribute, key string, op filter.Operator, value any) (T, error)
	Present(attribute *core.Attribute, key string) (T, error)
	And(left, right T) (T, error)
	Or(left, right T) (T, error)
	Not(operand T) (T, error)
	ValuePath(attribute *core.Attribute, key string, valueFilter func() (T, error)) (T, error)
}
