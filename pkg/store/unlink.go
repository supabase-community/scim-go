package store

import (
	"slices"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
)

func Unlink(schemas core.Schemas, object core.Object, attribute string) {
	for _, reference := range ReferencesOf(schemas) {
		if reference.Soft() && reference.Name == attribute {
			reference.holder(object, false).Remove(attribute)
		}
	}
	prune(schemas, object)
}

// RFC 7643 Section 3: schemas lists the extensions in use, so an emptied extension leaves it.
func prune(schemas core.Schemas, object core.Object) {
	for _, extension := range schemas.Extensions() {
		if holder := value.AsObject(object.Get(string(extension.ID))); holder != nil && len(holder) == 0 {
			object.Remove(string(extension.ID))
			listed, _ := object.Get("schemas").([]any)
			object.Set("schemas", slices.DeleteFunc(listed, func(urn any) bool { return urn == string(extension.ID) }))
		}
	}
}
