package server

import (
	"maps"
	"math"
	"reflect"

	"github.com/supabase-community/scim-go/pkg/core"
)

type Accessor[T any] func(item T) any

func normalized[T any](accessor Accessor[T]) Accessor[T] {
	if accessor == nil {
		return nil
	}
	return func(item T) any { return normalize(accessor(item)) }
}

func normalize(value any) any {
	v := reflect.ValueOf(value)
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Invalid:
		return nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if n := v.Uint(); n <= math.MaxInt64 {
			return int64(n)
		}
		return float64(v.Uint())
	case reflect.Float32, reflect.Float64:
		return v.Float()
	case reflect.String:
		return v.String()
	case reflect.Bool:
		return v.Bool()
	}
	return v.Interface()
}

type accessorSet[T Entity] map[*core.Attribute]Accessor[T]

// commonAccessors resolves id, externalId, and meta.*, per RFC 7643, Section 3.1, so they are filterable and sortable like any other attribute.
func commonAccessors[T Entity]() accessorSet[T] {
	id, _ := core.CommonAttribute("id")
	externalID, _ := core.CommonAttribute("externalId")
	meta, _ := core.CommonAttribute("meta")

	return accessorSet[T]{
		id:                                func(item T) any { return item.ResourceID() },
		externalID:                        func(item T) any { return item.GetExternalID() },
		meta.SubAttribute("resourceType"): func(item T) any { return string(item.GetMeta().ResourceType) },
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
