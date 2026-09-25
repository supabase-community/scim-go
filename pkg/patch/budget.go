package patch

import (
	"strconv"

	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type budget struct {
	max   int
	spent int
}

func (b *budget) charge(evaluations int) error {
	b.spent += evaluations
	if b.max > 0 && b.spent > b.max {
		return scimerrors.ErrTooLarge("the value filters of the request check more than " + strconv.Itoa(b.max) + " clauses")
	}
	return nil
}
