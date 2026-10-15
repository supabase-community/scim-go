package filter_test

import (
	"fmt"

	"github.com/supabase-community/scim-go/pkg/filter"
)

func ExampleParse() {
	node, err := filter.Parse(`userName eq "bjensen"`)
	if err != nil {
		panic(err)
	}
	fmt.Println(node.Attribute(), node.Operator(), node.Value())
	// Output: userName eq bjensen
}
