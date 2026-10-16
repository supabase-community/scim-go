package scimtest

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
)

func idOf(object core.Object) string {
	id, _ := object.Get("id").(string)
	return id
}

func idsOf(objects []core.Object) []string {
	ids := make([]string, 0, len(objects))
	for _, object := range objects {
		ids = append(ids, idOf(object))
	}
	return ids
}

func metaOf(object core.Object) map[string]any {
	meta, _ := object.Get("meta").(map[string]any)
	return meta
}

func objectOf(value any) map[string]any {
	object, _ := value.(map[string]any)
	return object
}

func elementsOf(value any) []map[string]any {
	list, _ := value.([]any)
	elements := make([]map[string]any, 0, len(list))
	for _, element := range list {
		if object, ok := element.(map[string]any); ok {
			elements = append(elements, object)
		}
	}
	return elements
}

func valuesIn(object core.Object, name string) []string {
	values := []string{}
	for _, element := range elementsOf(object.Get(name)) {
		if value, ok := element[valueSub].(string); ok {
			values = append(values, value)
		}
	}
	slices.Sort(values)
	return values
}

func elementWith(object core.Object, name, id string) map[string]any {
	for _, element := range elementsOf(object.Get(name)) {
		if element[valueSub] == id {
			return element
		}
	}
	return nil
}

func members(ids ...string) []any {
	values := make([]any, 0, len(ids))
	for _, id := range ids {
		values = append(values, map[string]any{valueSub: id})
	}
	return values
}

func isEmptyObject(value any) bool {
	object, ok := value.(map[string]any)
	return value == nil || ok && len(object) == 0
}

type holder struct {
	prefix string
	schema *core.Schema
	object core.Object
}

type leaf struct {
	path  string
	value any
}

func holders(schemas core.Schemas, object core.Object) []holder {
	found := []holder{{"", schemas.Base(), object}}
	for _, extension := range schemas.Extensions() {
		if nested := objectOf(object.Get(string(extension.ID))); nested != nil {
			found = append(found, holder{string(extension.ID) + ":", extension, nested})
		}
	}
	return found
}

func storedLeaves(schemas core.Schemas, object core.Object) []leaf {
	leaves := []leaf{}
	for _, h := range holders(schemas, object) {
		for _, attribute := range h.attributes() {
			if isStored(attribute) && !isReference(attribute) {
				leaves = append(leaves, leavesOf(h.prefix+attribute.Name, attribute, h.object.Get(attribute.Name))...)
			}
		}
	}
	return distinct(leaves)
}

func (h holder) attributes() core.Attributes {
	if h.prefix != "" {
		return h.schema.Attributes
	}
	return append(commonTargets(), h.schema.Attributes...)
}

func leavesOf(path string, attribute *core.Attribute, value any) []leaf {
	values := []any{value}
	if attribute.MultiValued {
		values, _ = value.([]any)
	}
	leaves := []leaf{}
	for _, v := range values {
		switch {
		case attribute.Type == core.TypeComplex:
			leaves = append(leaves, subLeaves(path, attribute, core.Object(objectOf(v)))...)
		case v != nil:
			leaves = append(leaves, leaf{path, v})
		}
	}
	return leaves
}

func subLeaves(path string, attribute *core.Attribute, element core.Object) []leaf {
	leaves := []leaf{}
	for _, sub := range attribute.SubAttributes {
		if found := element.Get(sub.Name); found != nil && isStored(sub) {
			leaves = append(leaves, leaf{path + "." + sub.Name, found})
		}
	}
	return leaves
}

func distinct(leaves []leaf) []leaf {
	seen := map[string]bool{}
	return slices.DeleteFunc(leaves, func(l leaf) bool {
		duplicate := seen[l.key()]
		seen[l.key()] = true
		return duplicate
	})
}

func (l leaf) literal() string {
	data, _ := json.Marshal(l.value)
	return string(data)
}

func (l leaf) key() string {
	return strings.ToLower(l.path) + " " + l.literal()
}

func keysOf(leaves []leaf) map[string]bool {
	keys := map[string]bool{}
	for _, l := range leaves {
		keys[l.key()] = true
	}
	return keys
}

func hasPath(keys map[string]bool, path string) bool {
	prefix := strings.ToLower(path) + " "
	for key := range keys {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}
