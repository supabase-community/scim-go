package patch

import (
	"strconv"

	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

const DefaultMaxWriteBytes = 8 << 20

type Option func(*budget)

type budget struct {
	max            int
	spent          int
	maxOutputBytes int
	outputSpent    int
}

// MaxFilterEvaluations caps the value filter clause checks of one request; costlier requests are refused, and zero lifts the cap.
func MaxFilterEvaluations(n int) Option {
	return func(b *budget) { b.max = n }
}

// MaxWriteBytes caps the bytes one request's add and replace operations may write; costlier requests are refused, and zero lifts the cap.
func MaxWriteBytes(n int) Option {
	return func(b *budget) { b.maxOutputBytes = n }
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
	if b.maxOutputBytes > 0 && b.outputSpent > b.maxOutputBytes {
		return scimerrors.ErrTooLarge("the value filters of the request would write more than " + strconv.Itoa(b.maxOutputBytes) + " bytes")
	}
	return nil
}
