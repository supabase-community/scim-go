package filter

var defaultGrammar = newGrammar(8192)

func Parse(text string) (*Node, error) {
	return defaultGrammar.Parse(text)
}
