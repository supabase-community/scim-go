package patch

import (
	"strconv"

	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

const maxOutputBytes = 8 << 20

type Option func(*limits)

type limits struct {
	maxEvaluations int
}

type budget struct {
	max         int
	spent       int
	outputSpent int
}

// MaxFilterEvaluations caps the value filter clause checks of one request; costlier requests are refused, and zero lifts the cap.
func MaxFilterEvaluations(n int) Option {
	return func(l *limits) { l.maxEvaluations = n }
}

func defaultLimits() limits {
	return limits{}
}

func (l limits) budget() *budget {
	return &budget{max: l.maxEvaluations}
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
