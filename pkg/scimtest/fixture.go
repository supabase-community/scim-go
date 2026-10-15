package scimtest

import (
	"crypto/rand"
	"encoding/json"
	"path"
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
)

type fixture func(schema *core.Schema, unique string) (core.Object, error)

func defaultFixtures() map[core.ResourceTypeName]fixture {
	return map[core.ResourceTypeName]fixture{
		"User":  golden(RFC7643MinimalUser),
		"Group": golden(RFC7643Group, "members"),
	}
}

func golden(name string, drop ...string) fixture {
	return func(schema *core.Schema, unique string) (core.Object, error) {
		data, err := files.ReadFile(path.Join("testdata", name))
		if err != nil {
			return nil, err
		}
		object, err := core.NewObject(json.RawMessage(data))
		if err != nil {
			return nil, err
		}
		for _, key := range drop {
			object.Remove(key)
		}
		uniquify(schema, object, unique)
		return object, nil
	}
}

func uniquify(schema *core.Schema, object core.Object, unique string) {
	for _, attribute := range schema.Attributes {
		if value, ok := object.Get(attribute.Name).(string); ok && isUnique(attribute) {
			object.Set(attribute.Name, uniquified(value, unique))
		}
	}
}

func uniquified(value, unique string) string {
	if local, domain, ok := strings.Cut(value, "@"); ok {
		return local + "+" + unique + "@" + domain
	}
	return value + "-" + unique
}

func isUnique(attribute *core.Attribute) bool {
	return attribute.Uniqueness != "" && attribute.Uniqueness != core.UniquenessNone
}

func newUnique() string {
	return strings.ToLower(rand.Text()[:12])
}
