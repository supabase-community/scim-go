package patch

type budget struct {
	max   int
	spent int
}

func (b *budget) spend() bool {
	b.spent++
	return !b.exceeded()
}

func (b *budget) exceeded() bool {
	return b.max > 0 && b.spent > b.max
}
