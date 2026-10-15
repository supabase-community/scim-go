package patch_test

import (
	"encoding/json"
	"fmt"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
)

func ExampleApply() {
	user := map[string]any{"userName": "old"}
	schemas := core.Schemas{core.NewSchema(core.SchemaUser).WithName("User").With(core.UserAttributes()...)}
	ops := []patch.Operation{
		{Op: patch.OpReplace, Path: "userName", Value: json.RawMessage(`"bjensen"`)},
	}
	if err := patch.Apply(user, ops, schemas); err != nil {
		panic(err)
	}
	fmt.Println(user["userName"])
	// Output: bjensen
}
