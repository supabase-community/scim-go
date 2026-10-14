package patch

import (
	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

// RFC 7644 Section 3.5.2: the "valuePath" rule allows specific values of a complex multi-valued attribute to be selected.
type filterState struct {
	match   predicate
	clauses int
	matched []bool
	parent  *core.Attribute
	budget  *budget
}

func (f *filterState) holders(container core.Object, name string) ([]core.Object, error) {
	elements, _ := container.Get(name).([]any)
	matched, err := f.matches(elements)
	if err != nil {
		return nil, err
	}
	f.matched = matched
	return members(elements, matched)
}

func (f *filterState) matches(elements []any) ([]bool, error) {
	if err := f.budget.charge(len(elements) * f.clauses); err != nil {
		return nil, err
	}
	matched := make([]bool, len(elements))
	for i, element := range elements {
		matched[i] = (isObject(element) || f.parent.Type != core.TypeComplex) && f.match(element)
	}
	return matched, nil
}

func (f *filterState) written(elements []any, kind Op, subAttribute string) func(int) bool {
	switch {
	case f.match != nil:
		return func(i int) bool { return f.matched[i] }
	case kind == OpAdd && subAttribute == "":
		return func(i int) bool { return i >= len(elements) }
	}
	return func(int) bool { return true }
}

func tally(matched []bool) int {
	count := 0
	for _, hit := range matched {
		if hit {
			count++
		}
	}
	return count
}

func members(elements []any, matched []bool) ([]core.Object, error) {
	holders := []core.Object{}
	for i, element := range elements {
		if member := value.AsObject(element); member != nil && (matched == nil || matched[i]) {
			holders = append(holders, member)
		}
	}
	if len(holders) == 0 {
		return nil, scimerrors.ErrNoTarget(`"path" matched no elements`)
	}
	return holders, nil
}
