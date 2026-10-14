package patch

import (
	"errors"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type remover struct {
	t *target
}

func (r remover) run() error {
	clear(r.t.indexes)
	err := r.values()
	if r.t.filter.match != nil && errors.Is(err, scimerrors.ErrNoTarget("")) {
		return nil // RFC 7644 Section 3.5.2.2: if nothing matched, no changes should be made and a success response should be returned.
	}
	return err
}

func (r remover) values() error {
	if r.t.filter.match != nil && r.t.path.SubAttribute == "" {
		return r.drop()
	}
	holders, err := r.t.holders(false)
	if err != nil {
		return err
	}
	for _, holder := range holders {
		if err := r.gateHolderRemoval(holder); err != nil {
			return err
		}
	}
	for _, holder := range holders {
		holder.Remove(r.t.key())
	}
	r.t.unassignEmptyExtension()
	return nil
}

// RFC 7643 Section 7: an assigned immutable attribute SHALL NOT be updated, including by removal.
func (r remover) gateHolderRemoval(holder core.Object) error {
	held := holder.Get(r.t.key())
	if err := gateRequired(r.t.attr, !value.IsUnassigned(held)); err != nil {
		return err
	}
	if err := gateImmutableWrite(r.t.attr, held, nil); err != nil {
		return err
	}
	return newParentGate(r.t.subParent(), holder).check(r.t.attr, held, nil)
}

func (r remover) drop() error {
	elements := r.t.elements()
	matched, err := r.t.filter.matches(elements)
	if err != nil {
		return err
	}
	count := tally(matched)
	if count == 0 {
		return scimerrors.ErrNoTarget(`"path" matched no elements`)
	}
	if err := gateRequired(r.t.attr, count == len(elements)); err != nil {
		return err
	}
	return r.t.replacer().rewrite(elements, matched, nil)
}
