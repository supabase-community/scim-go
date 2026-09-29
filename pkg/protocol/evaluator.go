package protocol

import "github.com/supabase-community/scim-go/pkg/filter"

type Evaluator[Output any] interface {
	Compare(attribute *Attribute, op filter.Operator, value any) (Output, error)
	Present(attribute *Attribute) (Output, error)
	And(left, right Output) (Output, error)
	Or(left, right Output) (Output, error)
	Not(operand Output) (Output, error)
	// ValuePath evaluates a valFilter; RFC 7644 Section 3.4.2.2: a valFilter uses sub-attributes of a parent attrPath.
	ValuePath(attribute *Attribute, valueFilter func() (Output, error)) (Output, error)
}

type accept struct{}

func (accept) Compare(*Attribute, filter.Operator, any) (struct{}, error) {
	return struct{}{}, nil
}

func (accept) Present(*Attribute) (struct{}, error) {
	return struct{}{}, nil
}

func (accept) And(_, _ struct{}) (struct{}, error) {
	return struct{}{}, nil
}

func (accept) Or(_, _ struct{}) (struct{}, error) {
	return struct{}{}, nil
}

func (accept) Not(struct{}) (struct{}, error) {
	return struct{}{}, nil
}

func (accept) ValuePath(_ *Attribute, valueFilter func() (struct{}, error)) (struct{}, error) {
	return valueFilter()
}
