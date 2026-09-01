%{
// This file is the source grammar for filter_grammar.go, generated via:
//
//	go tool goyacc -o filter_grammar.go -p scimFilter filter_grammar.y
//
// Each rule below implements exactly one production of the SCIM filter
// ABNF grammar, per RFC 7644, Section 3.4.2.2:
//
//	FILTER    = attrExp / logExp / valuePath / *1"not" "(" FILTER ")"
//	valuePath = attrPath "[" valFilter "]"
//	valFilter = attrExp / logExp / *1"not" "(" valFilter ")"
//	attrExp   = (attrPath SP "pr") / (attrPath SP compareOp SP compValue)
//	compValue = false / null / true / number / string
//	compareOp = "eq" / "ne" / "co" / "sw" / "ew" / "gt" / "lt" / "ge" / "le"
//	logExp    = FILTER SP ("and" / "or") SP FILTER
//	attrPath  = [URI ":"] ATTRNAME *1subAttr
package protocol
%}

%union {
	str    string
	filter Filter
	path   AttrPath
	value  any
}

%token <str> tATTRPATH
%token tAND tOR tNOT tPR
%token tEQ tNE tCO tSW tEW tGT tGE tLT tLE
%token tTRUE tFALSE tNULL
%token <str> tSTRING
%token <str> tNUMBER
%token tLPAREN tRPAREN tLBRACKET tRBRACKET

%type <filter> FILTER valFilter logExp valLogExp attrExp valuePath
%type <path> attrPath
%type <value> compValue
%type <str> compareOp

%left tOR
%left tAND
%right tNOT

%%

input:
	FILTER
	{
		scimFilterlex.(*filterLex).setResult($1)
	}
;

// FILTER = attrExp / logExp / valuePath / *1"not" "(" FILTER ")"
//
// The literal ABNF only allows bare "(" FILTER ")" grouping when preceded
// by "not", but RFC 7644's own Section 3.4.2.2 examples use bare grouping
// without "not" (e.g. `userType eq "Employee" and (emails co "example.com"
// or emails.value co "example.org")`). The bare-grouping alternative below
// extends the formal grammar to match those worked examples.
FILTER:
	attrExp
	{
		$$ = $1
	}
|	logExp
	{
		$$ = $1
	}
|	valuePath
	{
		$$ = $1
	}
|	tNOT tLPAREN FILTER tRPAREN
	{
		$$ = &NotExpr{Expr: $3}
	}
|	tLPAREN FILTER tRPAREN
	{
		$$ = $2
	}
;

// logExp = FILTER SP ("and" / "or") SP FILTER
logExp:
	FILTER tAND FILTER
	{
		$$ = &LogicalExpr{Op: "and", Left: $1, Right: $3}
	}
|	FILTER tOR FILTER
	{
		$$ = &LogicalExpr{Op: "or", Left: $1, Right: $3}
	}
;

// valuePath = attrPath "[" valFilter "]"
valuePath:
	attrPath tLBRACKET valFilter tRBRACKET
	{
		$$ = &ValuePathExpr{Attribute: $1.Attribute, Sub: $3}
	}
;

// valFilter = attrExp / logExp / *1"not" "(" valFilter ")"
//
// Same bare-grouping extension as FILTER above, and no valuePath
// alternative -- a valuePath filter may not itself contain a valuePath.
valFilter:
	attrExp
	{
		$$ = $1
	}
|	valLogExp
	{
		$$ = $1
	}
|	tNOT tLPAREN valFilter tRPAREN
	{
		$$ = &NotExpr{Expr: $3}
	}
|	tLPAREN valFilter tRPAREN
	{
		$$ = $2
	}
;

valLogExp:
	valFilter tAND valFilter
	{
		$$ = &LogicalExpr{Op: "and", Left: $1, Right: $3}
	}
|	valFilter tOR valFilter
	{
		$$ = &LogicalExpr{Op: "or", Left: $1, Right: $3}
	}
;

// attrExp = (attrPath SP "pr") / (attrPath SP compareOp SP compValue)
attrExp:
	attrPath tPR
	{
		$$ = &AttrExpr{Path: $1, Op: OpPresent}
	}
|	attrPath compareOp compValue
	{
		$$ = &AttrExpr{Path: $1, Op: FilterOp($2), Value: $3}
	}
;

// compareOp = "eq" / "ne" / "co" / "sw" / "ew" / "gt" / "lt" / "ge" / "le"
compareOp:
	tEQ
	{
		$$ = string(OpEqual)
	}
|	tNE
	{
		$$ = string(OpNotEqual)
	}
|	tCO
	{
		$$ = string(OpContains)
	}
|	tSW
	{
		$$ = string(OpStartsWith)
	}
|	tEW
	{
		$$ = string(OpEndsWith)
	}
|	tGT
	{
		$$ = string(OpGreaterThan)
	}
|	tGE
	{
		$$ = string(OpGreaterEqual)
	}
|	tLT
	{
		$$ = string(OpLessThan)
	}
|	tLE
	{
		$$ = string(OpLessEqual)
	}
;

// compValue = false / null / true / number / string
compValue:
	tFALSE
	{
		$$ = false
	}
|	tTRUE
	{
		$$ = true
	}
|	tNULL
	{
		$$ = nil
	}
|	tNUMBER
	{
		v, err := decodeFilterNumber($1)
		if err != nil {
			scimFilterlex.(*filterLex).setError(err)
		}
		$$ = v
	}
|	tSTRING
	{
		v, err := decodeFilterString($1)
		if err != nil {
			scimFilterlex.(*filterLex).setError(err)
		}
		$$ = v
	}
;

// attrPath = [URI ":"] ATTRNAME *1subAttr
attrPath:
	tATTRPATH
	{
		$$ = splitAttrPath($1)
	}
;

%%
