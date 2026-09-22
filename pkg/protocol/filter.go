package protocol

import (
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

func Filter[T any](schemas []*core.Schema, text string, v Evaluator[T]) (T, error) {
	var zero T
	node, err := filter.Parse(text)
	if err != nil {
		return zero, scimerrors.ErrInvalidFilter(err.Error())
	}
	return filter.Visit[T](&visitor[T]{schemas: schemas, inner: v}, node)
}
