package store

import (
	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
)

// ValueAttribute names the sub-attribute that holds a reference target's id, per RFC 7643, Section 2.3.7.
const ValueAttribute = "value"

// Reference is an attribute of a resource type that points at other resources, per RFC 7643, Section 2.3.7.
type Reference struct {
	core.ReferenceAttribute
	extension *core.Schema
}

func ReferencesOf(schemas core.Schemas) []Reference {
	references := []Reference{}
	for _, schema := range schemas {
		for _, attribute := range (core.Schemas{schema}).ReferenceAttributes() {
			reference := Reference{ReferenceAttribute: attribute}
			if schema != schemas.Base() {
				reference.extension = schema
			}
			references = append(references, reference)
		}
	}
	return references
}

// Inverse is true when the attribute is derived from another resource's attribute, like User groups, per RFC 7643, Section 4.1.2.
func (r Reference) Inverse() bool {
	return r.InverseOf != ""
}

// Soft is true for a single-valued reference like manager. Its value stays in the body and may name a resource this server does not hold, per RFC 7643, Section 2.3.7.
func (r Reference) Soft() bool {
	return !r.MultiValued && !r.Inverse()
}

func (r Reference) Types() []string {
	types := make([]string, len(r.Targets))
	for i, target := range r.Targets {
		types[i] = string(target)
	}
	return types
}

func (r Reference) holder(object core.Object, create bool) core.Object {
	if r.extension == nil {
		return object
	}
	key := string(r.extension.ID)
	holder := value.AsObject(object.Get(key))
	if holder == nil && create {
		holder = core.Object{}
		object.Set(key, holder)
	}
	return holder
}
