package protocol

import (
	"cmp"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"

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
func ParseFilter(expr string) (Filter, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, nil
	}

	tokens, err := lexFilter(expr)
	if err != nil {
		return nil, ErrInvalidFilter(err.Error())
	}

	p := &filterParser{tokens: tokens}
	filter, err := p.parseOr(true)
	if err != nil {
		return nil, ErrInvalidFilter(err.Error())
	}
	if p.peek().kind != tokEOF {
		return nil, ErrInvalidFilter("unexpected trailing input in filter")
	}
	return filter, nil
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

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokLParen
	tokRParen
	tokLBracket
	tokRBracket
	tokAnd
	tokOr
	tokNot
	tokPr
	tokEq
	tokNe
	tokCo
	tokSw
	tokEw
	tokGt
	tokGe
	tokLt
	tokLe
	tokString
	tokLiteral
	tokPath
)

type filterToken struct {
	kind tokenKind
	text string
}

func lexFilter(expr string) ([]filterToken, error) {
	var tokens []filterToken
	runes := []rune(expr)
	i, n := 0, len(runes)

	for i < n {
		c := runes[i]
		switch {
		case unicode.IsSpace(c):
			i++
		case c == '(':
			tokens = append(tokens, filterToken{kind: tokLParen})
			i++
		case c == ')':
			tokens = append(tokens, filterToken{kind: tokRParen})
			i++
		case c == '[':
			tokens = append(tokens, filterToken{kind: tokLBracket})
			i++
		case c == ']':
			tokens = append(tokens, filterToken{kind: tokRBracket})
			i++
		case c == '"':
			start := i
			i++
			for i < n && runes[i] != '"' {
				if runes[i] == '\\' && i+1 < n {
					i++
				}
				i++
			}
			if i >= n {
				return nil, fmt.Errorf("unterminated string literal")
			}
			i++
			tokens = append(tokens, filterToken{kind: tokString, text: string(runes[start:i])})
		default:
			start := i
			for i < n && isFilterWordChar(runes[i]) {
				i++
			}
			if i == start {
				return nil, fmt.Errorf("unexpected character %q", string(c))
			}
			tokens = append(tokens, classifyFilterWord(string(runes[start:i])))
		}
	}
	return tokens, nil
}

func isFilterWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '.' || r == ':' || r == '$'
}

func classifyFilterWord(word string) filterToken {
	switch strings.ToLower(word) {
	case "and":
		return filterToken{kind: tokAnd}
	case "or":
		return filterToken{kind: tokOr}
	case "not":
		return filterToken{kind: tokNot}
	case "pr":
		return filterToken{kind: tokPr}
	case "eq":
		return filterToken{kind: tokEq, text: string(OpEqual)}
	case "ne":
		return filterToken{kind: tokNe, text: string(OpNotEqual)}
	case "co":
		return filterToken{kind: tokCo, text: string(OpContains)}
	case "sw":
		return filterToken{kind: tokSw, text: string(OpStartsWith)}
	case "ew":
		return filterToken{kind: tokEw, text: string(OpEndsWith)}
	case "gt":
		return filterToken{kind: tokGt, text: string(OpGreaterThan)}
	case "ge":
		return filterToken{kind: tokGe, text: string(OpGreaterEqual)}
	case "lt":
		return filterToken{kind: tokLt, text: string(OpLessThan)}
	case "le":
		return filterToken{kind: tokLe, text: string(OpLessEqual)}
	case "true", "false", "null":
		return filterToken{kind: tokLiteral, text: word}
	}
	if _, err := strconv.ParseFloat(word, 64); err == nil {
		return filterToken{kind: tokLiteral, text: word}
	}
	return filterToken{kind: tokPath, text: word}
}

type filterParser struct {
	tokens []filterToken
	pos    int
}

func (p *filterParser) peek() filterToken {
	if p.pos < len(p.tokens) {
		return p.tokens[p.pos]
	}
	return filterToken{kind: tokEOF}
}

func (p *filterParser) next() filterToken {
	tok := p.peek()
	p.pos++
	return tok
}

func (p *filterParser) parseOr(allowValuePath bool) (Filter, error) {
	left, err := p.parseAnd(allowValuePath)
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokOr {
		p.next()
		right, err := p.parseAnd(allowValuePath)
		if err != nil {
			return nil, err
		}
		left = &LogicalExpr{Op: "or", Left: left, Right: right}
	}
	return left, nil
}

func (p *filterParser) parseAnd(allowValuePath bool) (Filter, error) {
	left, err := p.parseNot(allowValuePath)
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokAnd {
		p.next()
		right, err := p.parseNot(allowValuePath)
		if err != nil {
			return nil, err
		}
		left = &LogicalExpr{Op: "and", Left: left, Right: right}
	}
	return left, nil
}

func (p *filterParser) parseNot(allowValuePath bool) (Filter, error) {
	if p.peek().kind != tokNot {
		return p.parsePrimary(allowValuePath)
	}
	p.next()

	if p.peek().kind != tokLParen {
		return nil, fmt.Errorf(`expected "(" after "not"`)
	}
	p.next()

	inner, err := p.parseOr(allowValuePath)
	if err != nil {
		return nil, err
	}
	if p.peek().kind != tokRParen {
		return nil, fmt.Errorf(`expected ")" to close "not("`)
	}
	p.next()

	return &NotExpr{Expr: inner}, nil
}

func (p *filterParser) parsePrimary(allowValuePath bool) (Filter, error) {
	tok := p.peek()

	if tok.kind == tokLParen {
		p.next()
		inner, err := p.parseOr(allowValuePath)
		if err != nil {
			return nil, err
		}
		if p.peek().kind != tokRParen {
			return nil, fmt.Errorf(`expected ")"`)
		}
		p.next()
		return inner, nil
	}

	if tok.kind != tokPath {
		return nil, fmt.Errorf("expected an attribute path, got %q", tok.text)
	}
	p.next()
	path := splitAttrPath(tok.text)

	if allowValuePath && p.peek().kind == tokLBracket {
		p.next()
		sub, err := p.parseOr(false)
		if err != nil {
			return nil, err
		}
		if p.peek().kind != tokRBracket {
			return nil, fmt.Errorf(`expected "]"`)
		}
		p.next()
		return &ValuePathExpr{Attribute: path.Attribute, Sub: sub}, nil
	}

	op := p.next()
	if op.kind == tokPr {
		return &AttrExpr{Path: path, Op: OpPresent}, nil
	}

	switch op.kind {
	case tokEq, tokNe, tokCo, tokSw, tokEw, tokGt, tokGe, tokLt, tokLe:
		valTok := p.next()
		value, err := decodeFilterValue(valTok)
		if err != nil {
			return nil, err
		}
		return &AttrExpr{Path: path, Op: FilterOp(op.text), Value: value}, nil
	default:
		return nil, fmt.Errorf("expected a comparison operator, got %q", op.text)
	}
}

func decodeFilterValue(tok filterToken) (any, error) {
	switch tok.kind {
	case tokString, tokLiteral:
		var value any
		if err := json.Unmarshal([]byte(tok.text), &value); err != nil {
			return nil, fmt.Errorf("invalid comparison value %q: %w", tok.text, err)
		}
		return value, nil
	default:
		return nil, fmt.Errorf("expected a comparison value, got %q", tok.text)
	}
}
