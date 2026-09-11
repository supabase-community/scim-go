package filter

var defaultGrammar = newGrammar(8192)
var DefaultGrammar Grammar = defaultGrammar

func Parse(text string) (*Node, error) {
	return DefaultGrammar.Parse(text)
}
