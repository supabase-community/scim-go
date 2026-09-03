package scim

import (
	"cmp"
	"encoding/json"
	"strings"

	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

const (
	maxFilterLength = 2048
	maxFilterDepth  = 20
)

func filterResources[T any](items []T, text string) ([]T, error) {
	pred, err := compileFilter(text)
	if err != nil {
		return nil, err
	}

	matched := make([]T, 0, len(items))
	for _, item := range items {
		ok, err := matches(item, pred)
		if err != nil {
			return nil, err
		}
		if ok {
			matched = append(matched, item)
		}
	}
	return matched, nil
}

type predicate func(map[string]any) bool

func compileFilter(text string) (predicate, error) {
	if len(text) > maxFilterLength {
		return nil, protocol.ErrInvalidFilter("filter is too long")
	}
	if filterDepth(text) > maxFilterDepth {
		return nil, protocol.ErrInvalidFilter("filter nesting is too deep")
	}
	node, err := filter.Parse(text)
	if err != nil {
		return nil, protocol.ErrInvalidFilter(err.Error())
	}
	return filter.Visit[predicate](matchVisitor{}, node)
}

func filterDepth(text string) int {
	depth, deepest := 0, 0
	inString, escaped := false, false
	for _, r := range text {
		switch {
		case escaped:
			escaped = false
		case inString && r == '\\':
			escaped = true
		case r == '"':
			inString = !inString
		case inString:
		case r == '(':
			depth++
			if depth > deepest {
				deepest = depth
			}
		case r == ')':
			depth--
		}
	}
	return deepest
}

func matches[T any](v T, p predicate) (bool, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return false, err
	}
	var obj map[string]any
	if err := json.Unmarshal(data, &obj); err != nil {
		return false, err
	}
	return p(obj), nil
}

type matchVisitor struct{}

func (matchVisitor) VisitAnd(left, right predicate) (predicate, error) {
	return func(m map[string]any) bool { return left(m) && right(m) }, nil
}

func (matchVisitor) VisitOr(left, right predicate) (predicate, error) {
	return func(m map[string]any) bool { return left(m) || right(m) }, nil
}

func (matchVisitor) VisitNot(operand predicate) (predicate, error) {
	return func(m map[string]any) bool { return !operand(m) }, nil
}

func (matchVisitor) VisitEquals(attribute filter.AttrPath, value any) (predicate, error) {
	return compare(attribute, value, equalOp), nil
}

func (matchVisitor) VisitNotEquals(attribute filter.AttrPath, value any) (predicate, error) {
	equals := compare(attribute, value, equalOp)
	return func(m map[string]any) bool { return !equals(m) }, nil
}

func (matchVisitor) VisitContains(attribute filter.AttrPath, value any) (predicate, error) {
	return compare(attribute, lower(value), substringOp(strings.Contains)), nil
}

func (matchVisitor) VisitStartsWith(attribute filter.AttrPath, value any) (predicate, error) {
	return compare(attribute, lower(value), substringOp(strings.HasPrefix)), nil
}

func (matchVisitor) VisitEndsWith(attribute filter.AttrPath, value any) (predicate, error) {
	return compare(attribute, lower(value), substringOp(strings.HasSuffix)), nil
}

func (matchVisitor) VisitGreaterThan(attribute filter.AttrPath, value any) (predicate, error) {
	return compare(attribute, value, orderOp(func(c int) bool { return c > 0 })), nil
}

func (matchVisitor) VisitGreaterThanEquals(attribute filter.AttrPath, value any) (predicate, error) {
	return compare(attribute, value, orderOp(func(c int) bool { return c >= 0 })), nil
}

func (matchVisitor) VisitLessThan(attribute filter.AttrPath, value any) (predicate, error) {
	return compare(attribute, value, orderOp(func(c int) bool { return c < 0 })), nil
}

func (matchVisitor) VisitLessThanEquals(attribute filter.AttrPath, value any) (predicate, error) {
	return compare(attribute, value, orderOp(func(c int) bool { return c <= 0 })), nil
}

func (matchVisitor) VisitPresence(attribute filter.AttrPath) (predicate, error) {
	return compare(attribute, nil, func(got, _ any) bool { return present(got) }), nil
}

func (matchVisitor) VisitValuePath(path filter.AttrPath, subAttribute string, valueFilter func() (predicate, error)) (predicate, error) {
	inner, err := valueFilter()
	if err != nil {
		return nil, err
	}
	parts := segments(path)
	return func(m map[string]any) bool {
		for _, node := range flatten(lookupPath(m, parts)) {
			element, ok := node.(map[string]any)
			if !ok {
				continue
			}
			if !inner(element) {
				continue
			}
			if subAttribute == "" {
				return true
			}
			if _, ok := lookupCI(element, subAttribute); ok {
				return true
			}
		}
		return false
	}, nil
}

func compare(attribute filter.AttrPath, want any, op func(got, want any) bool) predicate {
	parts := segments(attribute)
	return func(m map[string]any) bool {
		for _, got := range flatten(lookupPath(m, parts)) {
			if op(got, want) {
				return true
			}
		}
		return false
	}
}

func equalOp(got, want any) bool {
	if gs, ws, ok := asStrings(got, want); ok {
		return strings.EqualFold(gs, ws)
	}
	return got == want
}

// substringOp compares against an already-lowercased want (see lower).
func substringOp(match func(s, substr string) bool) func(got, want any) bool {
	return func(got, want any) bool {
		gs, ws, ok := asStrings(got, want)
		return ok && match(strings.ToLower(gs), ws)
	}
}

func orderOp(keep func(c int) bool) func(got, want any) bool {
	return func(got, want any) bool {
		if gf, wf, ok := asFloats(got, want); ok {
			return keep(cmp.Compare(gf, wf))
		}
		if gs, ws, ok := asStrings(got, want); ok {
			return keep(cmp.Compare(gs, ws))
		}
		return false
	}
}

// lower lowercases a string literal for case-insensitive substring matching,
// once at compile time; non-strings pass through unchanged.
func lower(v any) any {
	if s, ok := v.(string); ok {
		return strings.ToLower(s)
	}
	return v
}

func asStrings(got, want any) (string, string, bool) {
	gs, gok := got.(string)
	ws, wok := want.(string)
	return gs, ws, gok && wok
}

func asFloats(got, want any) (float64, float64, bool) {
	gf, gok := got.(float64)
	wf, wok := want.(float64)
	return gf, wf, gok && wok
}

func present(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return x != ""
	case []any:
		return len(x) > 0
	case map[string]any:
		return len(x) > 0
	default:
		return true
	}
}

func segments(attr filter.AttrPath) []string {
	parts := make([]string, 0, 3)
	if attr.URI != "" {
		parts = append(parts, attr.URI)
	}
	parts = append(parts, attr.Name)
	if attr.Sub != "" {
		parts = append(parts, attr.Sub)
	}
	return parts
}

func lookupPath(m map[string]any, parts []string) []any {
	current := []any{m}
	for _, part := range parts {
		schemaURI := strings.Contains(part, ":")
		var next []any
		for _, node := range current {
			switch v := node.(type) {
			case map[string]any:
				switch val, ok := lookupCI(v, part); {
				case ok:
					next = append(next, val)
				case schemaURI:
					next = append(next, v)
				}
			case []any:
				for _, el := range v {
					if child, ok := el.(map[string]any); ok {
						if val, ok := lookupCI(child, part); ok {
							next = append(next, val)
						}
					}
				}
			}
		}
		current = next
	}
	return current
}

func flatten(vals []any) []any {
	nested := false
	for _, v := range vals {
		if _, ok := v.([]any); ok {
			nested = true
			break
		}
	}
	if !nested {
		return vals
	}

	out := make([]any, 0, len(vals))
	for _, v := range vals {
		if arr, ok := v.([]any); ok {
			out = append(out, arr...)
			continue
		}
		out = append(out, v)
	}
	return out
}

func lookupCI(m map[string]any, key string) (any, bool) {
	if v, ok := m[key]; ok {
		return v, true
	}
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return nil, false
}
