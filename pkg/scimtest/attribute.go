package scimtest

import (
	"slices"
	"testing"

	"github.com/supabase-community/scim-go/pkg/core"
)

func firstOf(attributes core.Attributes, match func(*core.Attribute) bool) *core.Attribute {
	if i := slices.IndexFunc(attributes, match); i >= 0 {
		return attributes[i]
	}
	return nil
}

func pathsOf(schema *core.Schema, match func(*core.Attribute) bool) map[string]*core.Attribute {
	paths := map[string]*core.Attribute{}
	for _, attribute := range schema.Attributes {
		if match(attribute) {
			paths[attribute.Name] = attribute
		}
		for _, sub := range attribute.SubAttributes {
			if match(sub) {
				paths[attribute.Name+"."+sub.Name] = sub
			}
		}
	}
	return paths
}

func extensionAttribute(schemas core.Schemas, match func(*core.Attribute) bool) (*core.Schema, *core.Attribute) {
	for _, extension := range schemas.Extensions() {
		if attribute := firstOf(extension.Attributes, match); attribute != nil {
			return extension, attribute
		}
	}
	return nil, nil
}

func filterableSubs(attribute *core.Attribute) core.Attributes {
	return slices.DeleteFunc(slices.Clone(attribute.SubAttributes), func(sub *core.Attribute) bool { return !isFilterable(sub) })
}

func isReadOnly(attribute *core.Attribute) bool {
	return attribute.Mutability == core.MutabilityReadOnly
}

func filled(subs core.Attributes) map[string]any {
	value := map[string]any{}
	for _, sub := range subs {
		value[sub.Name] = placeholder + " " + newUnique()
	}
	return value
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

func hasPrimary(attribute *core.Attribute) bool {
	value := attribute.SubAttribute(valueSub)
	return isWritableList(attribute) && value.Type == core.TypeString && value.Mutability == core.MutabilityReadWrite && attribute.SubAttribute(primarySub) != nil
}
