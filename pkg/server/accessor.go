package server

import (
	"maps"

	"github.com/supabase-community/scim-go/pkg/core"
)

type Accessor[T any] func(item T) any

type accessorSet[T Entity] map[*core.Attribute]Accessor[T]

// commonAccessors resolves id, externalId, and meta.*, per RFC 7643, Section 3.1, so they are filterable and sortable like any other attribute.
func commonAccessors[T Entity]() accessorSet[T] {
	id, _ := core.CommonAttribute("id")
	externalID, _ := core.CommonAttribute("externalId")
	meta, _ := core.CommonAttribute("meta")

	return accessorSet[T]{
		id:                                func(item T) any { return item.ResourceID() },
		externalID:                        func(item T) any { return item.GetExternalID() },
		meta.SubAttribute("resourceType"): func(item T) any { return item.GetMeta().ResourceType },
		meta.SubAttribute("created"):      func(item T) any { return item.GetMeta().Created },
		meta.SubAttribute("lastModified"): func(item T) any { return item.GetMeta().LastModified },
		meta.SubAttribute("location"):     func(item T) any { return item.GetMeta().Location },
		meta.SubAttribute("version"):      func(item T) any { return item.GetMeta().Version },
	}
}

func withCommonAccessors[T Entity](accessors accessorSet[T]) accessorSet[T] {
	merged := accessorSet[T]{}
	maps.Copy(merged, accessors)
	maps.Copy(merged, commonAccessors[T]())
	return merged
}
