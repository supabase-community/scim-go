package filter

var DefaultGrammar = func() *Grammar {
	g := New()
	g.MaxInputBytes = 8192
	return g
}()

func Parse(text string) (*Node, error) {
	return DefaultGrammar.Parse(text)
}
