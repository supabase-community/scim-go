// Package filter parses SCIM filters and PATCH paths, per RFC 7644 Sections 3.4.2.2 and 3.5.2.
package filter

import (
	"encoding/json"
	"errors"
	"regexp"

	"github.com/supabase-community/scim-go/pkg/filter/internal/peg"
)

var ErrInputTooLarge = errors.New("scim: filter exceeds the max input size")

var (
	reAttributeName = regexp.MustCompile(`\A[a-zA-Z][a-zA-Z0-9_\-]*`)
	reNumber        = regexp.MustCompile(`\A-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?(?:[eE][+-]?[0-9]+)?`)
	reString        = regexp.MustCompile(`\A"(?:[^"\\]|\\.)*"`)
	reSchemaURI     = regexp.MustCompile(`\A[A-Za-z][A-Za-z0-9.\-]*(?::[A-Za-z0-9.\-]+)*:`)
)

type Grammar interface {
	Parse(string) (*Node, error)
}

type grammar struct {
	maxInputBytes int
	filter        peg.Parser
	valueFilter   peg.Parser
	attrPath      peg.Parser
	path          peg.Parser
}

func New(maxInputBytes int) Grammar {
	return newGrammar(maxInputBytes)
}

func newGrammar(maxInputBytes int) *grammar {
	g := &grammar{maxInputBytes: maxInputBytes}

	filterRef := peg.Ref(&g.filter)
	valueRef := peg.Ref(&g.valueFilter)
	attrExp := g.attributeExpression()

	filterAtom := peg.Choice(g.parenGroup(filterRef), attrExp, g.valuePath(valueRef))
	// RFC 7644 Section 3.4.2.2: valFilter has no valuePath alternative.
	valueAtom := peg.Choice(g.parenGroup(valueRef), attrExp)

	g.filter = g.logExpr(filterAtom, filterRef)
	g.valueFilter = g.logExpr(valueAtom, valueRef)
	g.attrPath = g.attributePath()
	// RFC 7644 Section 3.5.2: PATH = attrPath / valuePath [subAttr]
	g.path = peg.Choice(
		peg.Sequence(g.valuePath(valueRef), peg.Optional(peg.Tag("sub_attribute", g.subAttribute()))),
		peg.Tag("path", g.attrPath),
	)

	return g
}

// Parse reads a SCIM filter and returns its AST, per RFC 7644 Section 3.4.2.2.
func (g *grammar) Parse(text string) (*Node, error) {
	return g.parse(text, g.filter)
}

func (g *grammar) parse(text string, p peg.Parser) (*Node, error) {
	raw, err := g.run(text, p)
	if err != nil {
		return nil, err
	}
	return NewNode(raw), nil
}

func (g *grammar) run(text string, p peg.Parser) (peg.ASTNode, error) {
	if g.exceedsMax(text) {
		return nil, ErrInputTooLarge
	}
	ctx := peg.NewContext(text)
	raw, err := p(ctx)
	if err != nil || ctx.Position() != len(text) {
		return nil, newParseError(text, ctx.Position())
	}
	return raw, nil
}

func (g *grammar) exceedsMax(text string) bool {
	return g.maxInputBytes > 0 && len(text) > g.maxInputBytes
}

// RFC 7644 Section 3.4.2.2: filters MUST be evaluated with "and" taking precedence over "or".
func (g *grammar) or(left, filter peg.Parser) peg.Parser {
	return binaryExpression(left, peg.Fold("or"), "or", filter)
}

func (g *grammar) and(atom, self peg.Parser) peg.Parser {
	return binaryExpression(atom, peg.Fold("and"), "and", self)
}

func (g *grammar) logExpr(atom, fallback peg.Parser) peg.Parser {
	var and peg.Parser
	and = g.and(atom, peg.Ref(&and))
	return g.or(and, fallback)
}

// RFC 7644 Section 3.4.2.2: Figure 2 puts SP between "not" and "(", which the ABNF omits.
func (g *grammar) parenGroup(sub peg.Parser) peg.Parser {
	group := peg.Sequence(peg.Str("("), sub, peg.Str(")"))
	return func(c *peg.Context) (peg.ASTNode, error) {
		start := c.Position()
		_, err := peg.Sequence(peg.Fold("not"), peg.Optional(peg.Space()))(c)
		negated := err == nil
		inner, err := group(c)
		if err != nil {
			c.Seek(start)
			return nil, err
		}
		if !negated {
			return inner, nil
		}
		return peg.Token{"not": inner}, nil
	}
}

// RFC 7644 Section 3.5.2: PATH = attrPath / valuePath [subAttr].
func (g *grammar) valuePath(valueFilter peg.Parser) peg.Parser {
	return peg.Sequence(
		peg.Tag("path", g.attributePath()),
		peg.Str("["),
		peg.Tag("value_filter", valueFilter),
		peg.Str("]"),
		peg.Optional(peg.Tag("sub_attribute", g.subAttribute())),
	)
}

func (g *grammar) attributeExpression() peg.Parser {
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

// RFC 7644 Section 3.4.2.2: attribute operators used in filters are case insensitive.
func (g *grammar) comparisonOperator() peg.Parser {
	return peg.Choice(
		peg.Fold("eq"), peg.Fold("ne"), peg.Fold("co"), peg.Fold("sw"), peg.Fold("ew"),
		peg.Fold("gt"), peg.Fold("lt"), peg.Fold("ge"), peg.Fold("le"),
	)
}

// RFC 7159 Section 3: the literal names MUST be lowercase.
func (g *grammar) comparisonValue() peg.Parser {
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

func (g *grammar) attributePath() peg.Parser {
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

// RFC 7644 Section 3.4.2.2: nameChar = "-" / "_" / DIGIT / ALPHA, without the "$" of RFC 7643 Section 2.1.
func (g *grammar) attributeName() peg.Parser {
	return peg.Match(reAttributeName)
}

func (g *grammar) subAttribute() peg.Parser {
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

func (g *grammar) schemaURI() peg.Parser {
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
		if _, err := peg.Sequence(peg.Space(), op, peg.Space())(c); err != nil {
			c.Seek(start)
			return l, nil
		}
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
