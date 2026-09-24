package protocol

import (
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

func Filter[Output any](schemas core.Schemas, text string, v Evaluator[Output]) (Output, error) {
	var zero Output
	node, err := filter.Parse(text)
	if err != nil {
		return zero, scimerrors.ErrInvalidFilter(err.Error())
	}
	return filter.Visit[Output](&visitor[Output]{schemas: schemas, inner: v}, node)
}
