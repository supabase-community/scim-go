// Package scim implements a parser for the SCIM RFC-7644 filter grammar.
package filter

import (
	"encoding/json"
	"errors"
	"regexp"
	"sync"

	"github.com/supabase-community/scim-go/pkg/filter/internal/peg"
)

var ErrInputTooLarge = errors.New("scim: filter exceeds MaxInputBytes")

var (
	reAttributeName = regexp.MustCompile(`\A[a-zA-Z][a-zA-Z0-9_\-]*`)
	reNumber        = regexp.MustCompile(`\A-?[0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?`)
	reString        = regexp.MustCompile(`\A"(?:[^"\\]|\\.)*"`)
	reSchemaURI     = regexp.MustCompile(`\A[A-Za-z][A-Za-z0-9.\-]*(?::[A-Za-z0-9.\-]+)*:`)
)

type Grammar struct {
	// MaxInputBytes, when > 0, makes Parse reject filters longer than this many
	// bytes before parsing (0 = unbounded, the default).
	MaxInputBytes int

	once        sync.Once
	filter      peg.Parser
	valueFilter peg.Parser
}

func New() *Grammar {
	g := &Grammar{}
	g.once.Do(g.build)
	return g
}

// Parse reads a SCIM filter (RFC 7644 3.4.2.2) and returns its AST
func (g *Grammar) Parse(text string) (*Node, error) {
	g.once.Do(g.build)
	raw, err := g.run(text, trailingSpace(g.filter))
	if err != nil {
		return nil, err
	}
	node := newNode(raw)
	if node == nil {
		return nil, NewParseError(text, len(text))
	}
	return node, nil
}

func (g *Grammar) run(text string, p peg.Parser) (peg.ASTNode, error) {
	if g.exceedsMax(text) {
		return nil, ErrInputTooLarge
	}
	ctx := peg.NewContext(text)
	raw, err := p(ctx)
	if err != nil || ctx.Position() != len(text) {
		return nil, NewParseError(text, ctx.Position())
	}
	return raw, nil
}

func trailingSpace(p peg.Parser) peg.Parser {
	return func(c *peg.Context) (peg.ASTNode, error) {
		raw, err := p(c)
		if err != nil {
			return nil, err
		}
		_, _ = peg.Space()(c)
		return raw, nil
	}
}

func (g *Grammar) exceedsMax(text string) bool {
	return g.MaxInputBytes > 0 && len(text) > g.MaxInputBytes
}

func (g *Grammar) build() {
	filterRef := peg.Ref(&g.filter)
	valueRef := peg.Ref(&g.valueFilter)
	attrExp := g.attributeExpression()

	// filterAtom = *1"not" "(" FILTER ")" / attrExp / valuePath
	filterAtom := peg.Choice(g.parenGroup(filterRef), attrExp, g.valuePath(valueRef))
	// valFilter atom omits valuePath: RFC forbids a nested value path here.
	valueAtom := peg.Choice(g.parenGroup(valueRef), attrExp)

	var andFilter, andValue peg.Parser
	andFilter = g.and(filterAtom, peg.Ref(&andFilter))
	andValue = g.and(valueAtom, peg.Ref(&andValue))

	g.filter = g.or(andFilter, filterRef)
	g.valueFilter = g.or(andValue, valueRef)
}

// logExp = FILTER SP "or" SP FILTER (loosest precedence, right-associative).
func (g *Grammar) or(left, filter peg.Parser) peg.Parser {
	return binaryExpression(left, peg.Fold("or"), "or", filter)
}

// logExp = FILTER SP "and" SP FILTER (binds tighter than "or").
func (g *Grammar) and(atom, self peg.Parser) peg.Parser {
	return binaryExpression(atom, peg.Fold("and"), "and", self)
}

// *1"not" "(" sub ")": an optional negation around a parenthesized sub-filter.
func (g *Grammar) parenGroup(sub peg.Parser) peg.Parser {
	return func(c *peg.Context) (peg.ASTNode, error) {
		start := c.Position()
		_, err := peg.Sequence(peg.Fold("not"), peg.Space())(c)
		negated := err == nil
		if _, err := peg.Str("(")(c); err != nil {
			c.Seek(start)
			return nil, err
		}
		inner, err := sub(c)
		if err != nil {
			c.Seek(start)
			return nil, err
		}
		if _, err := peg.Str(")")(c); err != nil {
			c.Seek(start)
			return nil, err
		}
		if !negated {
			return inner, nil
		}
		return peg.Token{"not": inner}, nil
	}
}

// valuePath = attrPath "[" valFilter "]" [subAttr]
func (g *Grammar) valuePath(valueFilter peg.Parser) peg.Parser {
	return peg.Sequence(
		peg.Tag("path", g.attributePath()),
		peg.Str("["),
		peg.Tag("value_filter", valueFilter),
		peg.Str("]"),
		peg.Optional(peg.Tag("sub_attribute", g.subAttribute())),
	)
}

// attrExp = (attrPath SP "pr") / (attrPath SP compareOp SP compValue)
func (g *Grammar) attributeExpression() peg.Parser {
	attrPath := g.attributePath()
	return peg.Choice(
		peg.Sequence(
			peg.Tag("attribute", attrPath), peg.Space(),
			peg.Tag("operator", peg.Fold("pr")),
		),
		peg.Sequence(
			peg.Tag("attribute", attrPath), peg.Space(),
			peg.Tag("operator", g.comparisonOperator()), peg.Space(),
			peg.Tag("value", g.comparisonValue()),
		),
	)
}

// compareOp = "eq" / "ne" / "co" / "sw" / "ew" / "gt" / "lt" / "ge" / "le"
// Operators are case-insensitive per RFC 7644.
func (g *Grammar) comparisonOperator() peg.Parser {
	return peg.Choice(
		peg.Fold("eq"), peg.Fold("ne"), peg.Fold("co"), peg.Fold("sw"), peg.Fold("ew"),
		peg.Fold("gt"), peg.Fold("lt"), peg.Fold("ge"), peg.Fold("le"),
	)
}

// compValue = false / null / true / number / string (JSON rules, RFC 7159).
// JSON literals are lowercase-only, unlike the case-insensitive operators.
func (g *Grammar) comparisonValue() peg.Parser {
	return peg.Choice(
		constant("false", false),
		constant("null", nil),
		constant("true", true),
		convert(peg.Match(reNumber), func(s string) (peg.ASTNode, error) {
			return json.Number(s), nil
		}),
		convert(peg.Match(reString), func(s string) (peg.ASTNode, error) {
			var out string
			err := json.Unmarshal([]byte(s), &out)
			return out, err
		}),
	)
}

// attrPath = [URI ":"] ATTRNAME *1subAttr
func (g *Grammar) attributePath() peg.Parser {
	attrName := g.attributeName()
	schemaURI := g.schemaURI()
	subAttr := g.subAttribute()
	return func(c *peg.Context) (peg.ASTNode, error) {
		start := c.Position()
		prefix := ""
		if uri, err := schemaURI(c); err == nil {
			prefix = uri.(string) + ":"
		}
		name, err := attrName(c)
		if err != nil {
			c.Seek(start)
			return nil, err
		}
		path := prefix + name.(string)
		if sub, err := subAttr(c); err == nil {
			path += "." + sub.(string)
		}
		return path, nil
	}
}

// ATTRNAME = ALPHA *(nameChar)
func (g *Grammar) attributeName() peg.Parser {
	return peg.Match(reAttributeName)
}

// subAttr = "." ATTRNAME
func (g *Grammar) subAttribute() peg.Parser {
	attrName := g.attributeName()
	return func(c *peg.Context) (peg.ASTNode, error) {
		start := c.Position()
		if _, err := peg.Str(".")(c); err != nil {
			return nil, err
		}
		name, err := attrName(c)
		if err != nil {
			c.Seek(start)
			return nil, err
		}
		return name, nil
	}
}

// URI ":" prefix on an attrPath. The trailing ":" is stripped from the value.
func (g *Grammar) schemaURI() peg.Parser {
	return convert(peg.Match(reSchemaURI), func(s string) (peg.ASTNode, error) {
		return s[:len(s)-1], nil
	})
}

func binaryExpression(left, op peg.Parser, name string, right peg.Parser) peg.Parser {
	return func(c *peg.Context) (peg.ASTNode, error) {
		l, err := left(c)
		if err != nil {
			return nil, err
		}
		start := c.Position()
		_, _ = peg.Space()(c)
		if _, err := op(c); err != nil {
			c.Seek(start)
			return l, nil
		}
		_, _ = peg.Space()(c)
		r, err := right(c)
		if err != nil {
			c.Seek(start)
			return l, nil
		}
		return peg.Token{"left": l, "operator": name, "right": r}, nil
	}
}

func constant(lit string, val peg.ASTNode) peg.Parser {
	return func(c *peg.Context) (peg.ASTNode, error) {
		if _, err := peg.Str(lit)(c); err != nil {
			return nil, err
		}
		return val, nil
	}
}

func convert(p peg.Parser, fn func(string) (peg.ASTNode, error)) peg.Parser {
	return func(c *peg.Context) (peg.ASTNode, error) {
		start := c.Position()
		val, err := p(c)
		if err != nil {
			return nil, err
		}
		out, err := fn(val.(string))
		if err != nil {
			c.Seek(start)
			return nil, err
		}
		return out, nil
	}
}
