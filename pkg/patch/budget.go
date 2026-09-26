package patch

import (
	"strconv"

	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

const maxOutputBytes = 8 << 20

type budget struct {
	max         int
	spent       int
	outputSpent int
}

func (b *budget) charge(evaluations int) error {
	b.spent += evaluations
	if b.max > 0 && b.spent > b.max {
		return scimerrors.ErrTooLarge("the value filters of the request check more than " + strconv.Itoa(b.max) + " clauses")
	}
	return nil
}

func (b *budget) chargeBytes(n int) error {
	b.outputSpent += n
	if b.outputSpent > maxOutputBytes {
		return scimerrors.ErrTooLarge("the value filters of the request would write more than " + strconv.Itoa(maxOutputBytes) + " bytes")
	}
	return nil
}
