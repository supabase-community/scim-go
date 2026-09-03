package filter

import (
	"strings"
	"sync"
	"testing"
)

func TestNestingIsLinearTime(t *testing.T) {
	g := New()
	for _, depth := range []int{1000, 2000, 4000, 8000} {
		in := strings.Repeat("(", depth) + `a eq "1"` + strings.Repeat(")", depth)
		node, err := g.Parse(in)
		if err != nil || node == nil {
			t.Fatalf("depth %d: node=%v err=%v", depth, node, err)
		}
	}
}

func TestConcurrentParse(t *testing.T) {
	g := &Grammar{}
	inputs := []string{
		`userName eq "bjensen"`,
		`emails[type eq "work" and primary eq true].value`,
		`not (a eq "1" or b pr) and urn:ietf:params:scim:schemas:core:2.0:User:x eq "q"`,
	}
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			in := inputs[i%len(inputs)]
			if node, err := g.Parse(in); err != nil || node == nil {
				t.Errorf("parse %q: node=%v err=%v", in, node, err)
			}
		}(i)
	}
	wg.Wait()
}
