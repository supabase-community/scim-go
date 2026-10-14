package patch

import (
	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

type replacer struct {
	t *target
}

func (r replacer) selectsValues(incoming any) bool {
	switch {
	case r.t.filter.match == nil || r.t.path.SubAttribute != "":
		return false
	case r.t.parent != permissiveAttr:
		return isSimple(r.t.parent)
	}
	elements := r.t.elements()
	return len(elements) > 0 && !isObject(elements[0]) && !isObject(incoming)
}

// RFC 7644 Section 3.5.2.3: all matching record values SHALL be replaced.
func (r replacer) replace(kind Op, incoming any) error {
	if kind != OpReplace {
		return scimerrors.ErrInvalidPath(`"add" cannot target values selected by a filter`)
	}
	if _, ok := r.typeOf(incoming).Coerce(incoming); !ok || incoming == nil {
		return scimerrors.ErrInvalidValue(`"value" must be a single value of the attribute's type`)
	}
	elements := r.t.elements()
	matched, err := r.t.filter.matches(elements)
	if err != nil {
		return err
	}
	count := tally(matched)
	if count == 0 {
		return scimerrors.ErrNoTarget(`"path" matched no elements`)
	}
	if err := r.t.chargeOutput(1, incoming); err != nil {
		return err
	}
	return r.rewrite(elements, matched, incoming)
}

func (r replacer) typeOf(incoming any) *core.Attribute {
	if r.t.attr == permissiveAttr {
		return inferred(incoming)
	}
	return r.t.attr
}

func (r replacer) rewrite(elements []any, matched []bool, incoming any) error {
	after := r.substitute(elements, matched, incoming)
	if err := gateImmutableWrite(r.t.attr, elements, after); err != nil {
		return err
	}
	container, _ := r.t.container(false)
	container.Set(r.t.path.Name, after)
	clear(r.t.indexes)
	return nil
}

func (r replacer) substitute(elements []any, matched []bool, incoming any) []any {
	after := elements[:0]
	if r.t.attr.Mutability == core.MutabilityImmutable {
		after = make([]any, 0, len(elements))
	}
	placed := incoming == nil || r.keeps(elements, matched, incoming)
	for i, element := range elements {
		switch {
		case !matched[i]:
			after = append(after, element)
		case !placed:
			after = append(after, incoming)
			placed = true
		}
	}
	return after
}

func (r replacer) keeps(elements []any, matched []bool, incoming any) bool {
	for i, element := range elements {
		if !matched[i] && value.Equal(r.t.attr, element, incoming) {
			return true
		}
	}
	return false
}
