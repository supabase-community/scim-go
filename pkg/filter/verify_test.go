package filter_test

import (
	"sync"
	"testing"

	"github.com/supabase-community/scim-go/pkg/filter"
)

func TestConcurrentParse(t *testing.T) {
	g := filter.New(0)
	inputs := []string{
		`userName eq "bjensen"`,
		`emails[type eq "work" and primary eq true]`,
		`not (a eq "1" or b pr) and urn:ietf:params:scim:schemas:core:2.0:User:x eq "q"`,
	}
	var wg sync.WaitGroup
	for i := range 200 {
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
