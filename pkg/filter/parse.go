package filter

var defaultGrammar = New()

// Parse reads a SCIM filter (RFC 7644 3.4.2.2) and returns its AST, or a
// *ParseError on malformed input. It uses a shared unbounded Grammar; callers
// that need an input-size backstop should build their own Grammar with
// MaxInputBytes set and bound request input at the transport layer.
func Parse(text string) (*Node, error) {
	return defaultGrammar.Parse(text)
}
