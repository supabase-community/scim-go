package protocol

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// filterToken is one lexical token of a SCIM filter expression, tagged with
// the goyacc token constant generated from filter_grammar.y.
type filterToken struct {
	kind int
	text string
}

// filterLex adapts a pre-scanned token stream to goyacc's yyLexer interface
// (Lex/Error), as required by scimFilterParse in filter_grammar.go.
type filterLex struct {
	tokens []filterToken
	pos    int
	result Filter
	err    error
}

func newFilterLex(tokens []filterToken) *filterLex {
	return &filterLex{tokens: tokens}
}

func (l *filterLex) setResult(f Filter) {
	l.result = f
}

func (l *filterLex) setError(err error) {
	if l.err == nil {
		l.err = err
	}
}

func (l *filterLex) Lex(lval *scimFilterSymType) int {
	if l.pos >= len(l.tokens) {
		return 0
	}
	tok := l.tokens[l.pos]
	l.pos++
	lval.str = tok.text
	return tok.kind
}

func (l *filterLex) Error(s string) {
	if l.err != nil {
		return
	}
	if l.pos == 0 || l.pos-1 >= len(l.tokens) {
		l.err = fmt.Errorf("%s: unexpected end of filter", s)
		return
	}
	if tok := l.tokens[l.pos-1]; tok.text != "" {
		l.err = fmt.Errorf("%s near %q", s, tok.text)
	} else {
		l.err = fmt.Errorf("%s near %q", s, filterTokenName(tok.kind))
	}
}

// filterTokenName gives a human-readable name for a fixed-keyword token, for
// use in error messages. Tokens that carry their own text (attribute paths,
// strings, numbers) are reported using that text instead; see Error above.
func filterTokenName(kind int) string {
	switch kind {
	case tAND:
		return "and"
	case tOR:
		return "or"
	case tNOT:
		return "not"
	case tPR:
		return "pr"
	case tEQ:
		return "eq"
	case tNE:
		return "ne"
	case tCO:
		return "co"
	case tSW:
		return "sw"
	case tEW:
		return "ew"
	case tGT:
		return "gt"
	case tGE:
		return "ge"
	case tLT:
		return "lt"
	case tLE:
		return "le"
	case tTRUE:
		return "true"
	case tFALSE:
		return "false"
	case tNULL:
		return "null"
	case tLPAREN:
		return "("
	case tRPAREN:
		return ")"
	case tLBRACKET:
		return "["
	case tRBRACKET:
		return "]"
	default:
		return "token"
	}
}

// lexFilterTokens tokenizes a SCIM filter expression, per RFC 7644, Section 3.4.2.2.
func lexFilterTokens(expr string) ([]filterToken, error) {
	var tokens []filterToken
	runes := []rune(expr)
	i, n := 0, len(runes)

	for i < n {
		c := runes[i]
		switch {
		case unicode.IsSpace(c):
			i++
		case c == '(':
			tokens = append(tokens, filterToken{kind: tLPAREN})
			i++
		case c == ')':
			tokens = append(tokens, filterToken{kind: tRPAREN})
			i++
		case c == '[':
			tokens = append(tokens, filterToken{kind: tLBRACKET})
			i++
		case c == ']':
			tokens = append(tokens, filterToken{kind: tRBRACKET})
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
			tokens = append(tokens, filterToken{kind: tSTRING, text: string(runes[start:i])})
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
		return filterToken{kind: tAND}
	case "or":
		return filterToken{kind: tOR}
	case "not":
		return filterToken{kind: tNOT}
	case "pr":
		return filterToken{kind: tPR}
	case "eq":
		return filterToken{kind: tEQ}
	case "ne":
		return filterToken{kind: tNE}
	case "co":
		return filterToken{kind: tCO}
	case "sw":
		return filterToken{kind: tSW}
	case "ew":
		return filterToken{kind: tEW}
	case "gt":
		return filterToken{kind: tGT}
	case "ge":
		return filterToken{kind: tGE}
	case "lt":
		return filterToken{kind: tLT}
	case "le":
		return filterToken{kind: tLE}
	case "true":
		return filterToken{kind: tTRUE}
	case "false":
		return filterToken{kind: tFALSE}
	case "null":
		return filterToken{kind: tNULL}
	}
	if _, err := strconv.ParseFloat(word, 64); err == nil {
		return filterToken{kind: tNUMBER, text: word}
	}
	return filterToken{kind: tATTRPATH, text: word}
}

func decodeFilterNumber(text string) (float64, error) {
	v, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid comparison value %q: %w", text, err)
	}
	return v, nil
}

func decodeFilterString(text string) (string, error) {
	var v string
	if err := json.Unmarshal([]byte(text), &v); err != nil {
		return "", fmt.Errorf("invalid comparison value %q: %w", text, err)
	}
	return v, nil
}
