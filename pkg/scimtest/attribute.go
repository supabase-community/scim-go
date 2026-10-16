package scimtest

import (
	"slices"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
)

func find(schema *core.Schema, match func(*core.Attribute) bool) *core.Attribute {
	if i := slices.IndexFunc(schema.Attributes, match); i >= 0 {
		return schema.Attributes[i]
	}
	return nil
}

func isReadOnly(attribute *core.Attribute) bool {
	return attribute.Mutability == core.MutabilityReadOnly
}

func readOnlyValue(attribute *core.Attribute, marker string) any {
	value := any(marker)
	if attribute.Type == core.TypeComplex {
		value = map[string]any{valueSub: marker}
	}
	if attribute.MultiValued {
		value = []any{value}
	}
	return value
}

func (r referencing) patchable(t *testing.T) {
	t.Helper()
	supports(t, "patch", r.source.config.Patch.Supported)
}

func required(object core.Object) func(*core.Attribute) bool {
	return func(a *core.Attribute) bool { return a.Required && a.Name != "id" && object.Has(a.Name) }
}

func isFilterable(attribute *core.Attribute) bool {
	return attribute.Type == core.TypeString && !attribute.MultiValued &&
		attribute.Mutability == core.MutabilityReadWrite && len(attribute.CanonicalValues) == 0 &&
		(attribute.Returned == core.ReturnedDefault || attribute.Returned == core.ReturnedAlways)
}

func isUnique(attribute *core.Attribute) bool {
	return attribute.Uniqueness != "" && attribute.Uniqueness != core.UniquenessNone
}

func isWritableList(attribute *core.Attribute) bool {
	value := attribute.SubAttribute(valueSub)
	return attribute.MultiValued && attribute.Mutability == core.MutabilityReadWrite && value != nil
}
