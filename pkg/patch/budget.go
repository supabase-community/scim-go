package patch

import (
	"strconv"

	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

const defaultMaxWriteBytes = 8 << 20

type Option func(*limits)

type limits struct {
	maxEvaluations int
	maxWriteBytes  int
}

type budget struct {
	max            int
	spent          int
	maxOutputBytes int
	outputSpent    int
}

// MaxFilterEvaluations caps the value filter clause checks of one request; costlier requests are refused, and zero lifts the cap.
func MaxFilterEvaluations(n int) Option {
	return func(l *limits) { l.maxEvaluations = n }
}

// MaxWriteBytes caps the bytes a request's value filters may write; costlier requests are refused, and zero lifts the cap.
func MaxWriteBytes(n int) Option {
	return func(l *limits) { l.maxWriteBytes = n }
}

func defaultLimits() limits {
	return limits{maxWriteBytes: defaultMaxWriteBytes}
}

func (l limits) budget() *budget {
	return &budget{max: l.maxEvaluations, maxOutputBytes: l.maxWriteBytes}
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
