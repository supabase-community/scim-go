package patch

import (
	"errors"
	"strconv"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

var errSkip = errors.New("scim: skip readOnly attribute")

type guard struct{}

func (guard) permits(attr *core.Attribute, present bool) error {
	switch attr.Mutability {
	case core.MutabilityReadOnly:
		return errSkip
	case core.MutabilityImmutable:
		if present {
			return scimerrors.ErrMutability(strconv.Quote(attr.Name) + " is immutable")
		}
	}
	return nil
}

func (guard) skipOrFail(err error) error {
	if errors.Is(err, errSkip) {
		return nil
	}
	return err
}
