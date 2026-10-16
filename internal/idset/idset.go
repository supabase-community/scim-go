package idset

type Set map[string]struct{}

func (s Set) Add(id string) {
	s[id] = struct{}{}
}

func (s Set) Has(id string) bool {
	_, ok := s[id]
	return ok
}
