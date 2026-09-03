package peg

type Context struct {
	stream   string
	position int
}

func NewContext(stream string) *Context {
	return &Context{stream: stream, position: 0}
}

func (c *Context) Position() int {
	return c.position
}

func (c *Context) Seek(position int) {
	c.position = position
}
