package filter

var DefaultGrammar = func() *Grammar {
	return New(8192)
}()

func Parse(text string) (*Node, error) {
	return DefaultGrammar.Parse(text)
}
