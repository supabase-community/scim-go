//go:generate go tool goyacc -o filter_grammar.go -p scimFilter filter_grammar.y

package protocol

import (
	"cmp"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"mokhan.ca/go/scim/pkg/core"
)

// FilterOp is a comparison or presence operator, per RFC 7644, Section 3.4.2.2.
type FilterOp string

const (
	OpEqual        FilterOp = "eq"
	OpNotEqual     FilterOp = "ne"
	OpContains     FilterOp = "co"
	OpStartsWith   FilterOp = "sw"
	OpEndsWith     FilterOp = "ew"
	OpGreaterThan  FilterOp = "gt"
	OpGreaterEqual FilterOp = "ge"
	OpLessThan     FilterOp = "lt"
	OpLessEqual    FilterOp = "le"
	OpPresent      FilterOp = "pr"
)

type AttrPath struct {
	URI       core.SchemaURI
	Attribute string
	SubAttr   string
}

type Filter interface{ isFilter() }

type AttrExpr struct {
	Path  AttrPath
	Op    FilterOp
	Value any
}

type LogicalExpr struct {
	Op          string
	Left, Right Filter
}

type NotExpr struct{ Expr Filter }

// ValuePathExpr is the valuePath filter of RFC 7644, Section 3.4.2.2.
type ValuePathExpr struct {
	Attribute string
	Sub       Filter
}

func (*AttrExpr) isFilter()      {}
func (*LogicalExpr) isFilter()   {}
func (*NotExpr) isFilter()       {}
func (*ValuePathExpr) isFilter() {}

// ParseFilter parses a SCIM filter expression, per RFC 7644, Section 3.4.2.2.
// Parsing is driven by the goyacc grammar in filter_grammar.y.
func ParseFilter(expr string) (Filter, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, nil
	}

	tokens, err := lexFilterTokens(expr)
	if err != nil {
		return nil, ErrInvalidFilter(err.Error())
	}

	lex := newFilterLex(tokens)
	if scimFilterParse(lex) != 0 || lex.err != nil {
		if lex.err != nil {
			return nil, ErrInvalidFilter(lex.err.Error())
		}
		return nil, ErrInvalidFilter("invalid filter expression")
	}
	return lex.result, nil
}

func Matches(f Filter, resource any) (bool, error) {
	if f == nil {
		return true, nil
	}

	doc, err := toDocument(resource)
	if err != nil {
		return false, err
	}
	return matchDocument(f, doc)
}

func toDocument(resource any) (map[string]any, error) {
	body, err := json.Marshal(resource)
	if err != nil {
		return nil, err
	}

	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func matchDocument(f Filter, doc map[string]any) (bool, error) {
	switch expr := f.(type) {
	case *LogicalExpr:
		left, err := matchDocument(expr.Left, doc)
		if err != nil {
			return false, err
		}
		if expr.Op == "and" && !left {
			return false, nil
		}
		if expr.Op == "or" && left {
			return true, nil
		}
		return matchDocument(expr.Right, doc)
	case *NotExpr:
		matched, err := matchDocument(expr.Expr, doc)
		if err != nil {
			return false, err
		}
		return !matched, nil
	case *ValuePathExpr:
		items, ok := lookup(doc, expr.Attribute).([]any)
		if !ok {
			return false, nil
		}
		for _, item := range items {
			element, ok := item.(map[string]any)
			if !ok {
				continue
			}
			matched, err := matchDocument(expr.Sub, element)
			if err != nil {
				return false, err
			}
			if matched {
				return true, nil
			}
		}
		return false, nil
	case *AttrExpr:
		return matchAttr(expr, doc), nil
	default:
		return false, fmt.Errorf("scim: unknown filter node %T", f)
	}
}

func matchAttr(expr *AttrExpr, doc map[string]any) bool {
	value, present := lookupCI(doc, expr.Path.Attribute)

	if expr.Path.SubAttr == "" {
		return compareValue(expr.Op, value, present, expr.Value)
	}

	switch v := value.(type) {
	case []any:
		for _, item := range v {
			element, ok := item.(map[string]any)
			if !ok {
				continue
			}
			sub, ok := lookupCI(element, expr.Path.SubAttr)
			if compareValue(expr.Op, sub, ok, expr.Value) {
				return true
			}
		}
		return false
	case map[string]any:
		sub, ok := lookupCI(v, expr.Path.SubAttr)
		return compareValue(expr.Op, sub, ok, expr.Value)
	default:
		return false
	}
}

func compareValue(op FilterOp, actual any, present bool, expected any) bool {
	if op == OpPresent {
		return present && !isEmptyValue(actual)
	}
	if !present {
		return false
	}

	switch op {
	case OpEqual:
		return equalValues(actual, expected)
	case OpNotEqual:
		return !equalValues(actual, expected)
	case OpContains, OpStartsWith, OpEndsWith:
		a, aok := actual.(string)
		e, eok := expected.(string)
		if !aok || !eok {
			return false
		}
		a, e = strings.ToLower(a), strings.ToLower(e)
		switch op {
		case OpContains:
			return strings.Contains(a, e)
		case OpStartsWith:
			return strings.HasPrefix(a, e)
		case OpEndsWith:
			return strings.HasSuffix(a, e)
		}
		return false
	case OpGreaterThan, OpGreaterEqual, OpLessThan, OpLessEqual:
		return compareOrdered(op, actual, expected)
	default:
		return false
	}
}

func equalValues(actual, expected any) bool {
	switch e := expected.(type) {
	case string:
		a, ok := actual.(string)
		return ok && strings.EqualFold(a, e)
	case float64:
		a, ok := actual.(float64)
		return ok && a == e
	case bool:
		a, ok := actual.(bool)
		return ok && a == e
	case nil:
		return actual == nil
	default:
		return reflect.DeepEqual(actual, expected)
	}
}

func compareOrdered(op FilterOp, actual, expected any) bool {
	if a, ok := actual.(float64); ok {
		if e, ok := expected.(float64); ok {
			return applyOrder(op, cmp.Compare(a, e))
		}
	}

	a, aok := actual.(string)
	e, eok := expected.(string)
	if !aok || !eok {
		return false
	}
	if at, err := time.Parse(time.RFC3339, a); err == nil {
		if et, err := time.Parse(time.RFC3339, e); err == nil {
			return applyOrder(op, at.Compare(et))
		}
	}
	return applyOrder(op, strings.Compare(a, e))
}

func applyOrder(op FilterOp, order int) bool {
	switch op {
	case OpGreaterThan:
		return order > 0
	case OpGreaterEqual:
		return order >= 0
	case OpLessThan:
		return order < 0
	case OpLessEqual:
		return order <= 0
	default:
		return false
	}
}

func isEmptyValue(v any) bool {
	switch val := v.(type) {
	case nil:
		return true
	case string:
		return val == ""
	case []any:
		return len(val) == 0
	case map[string]any:
		return len(val) == 0
	default:
		return false
	}
}

func lookup(doc map[string]any, key string) any {
	value, _ := lookupCI(doc, key)
	return value
}

func lookupCI(doc map[string]any, key string) (any, bool) {
	if v, ok := doc[key]; ok {
		return v, true
	}
	for k, v := range doc {
		if strings.EqualFold(k, key) {
			return v, true
		}
	}
	return nil, false
}

func splitAttrPath(token string) AttrPath {
	uri := core.SchemaURI("")
	rest := token
	if idx := strings.LastIndex(token, ":"); idx != -1 {
		uri = core.SchemaURI(token[:idx])
		rest = token[idx+1:]
	}
	attr, sub, _ := strings.Cut(rest, ".")
	return AttrPath{URI: uri, Attribute: attr, SubAttr: sub}
}
