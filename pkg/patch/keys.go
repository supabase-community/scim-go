package patch

import (
	"strings"
	"unicode"

	"github.com/supabase-community/scim-go/pkg/core"
)

type keys struct {
	object core.Object
	folded map[string]string
}

func newKeys(object core.Object) *keys {
	k := &keys{object: object, folded: make(map[string]string, len(object))}
	for key := range object {
		k.add(key)
	}
	return k
}

func (k *keys) resolve(name string) string {
	if _, ok := k.object[name]; ok {
		return name
	}
	if key, ok := k.folded[fold(name)]; ok {
		return key
	}
	return name
}

func (k *keys) add(key string) {
	folded := fold(key)
	if existing, ok := k.folded[folded]; !ok || key < existing {
		k.folded[folded] = key
	}
}

func fold(name string) string {
	var folded strings.Builder
	folded.Grow(len(name))
	for _, r := range name {
		smallest := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			smallest = min(smallest, next)
		}
		folded.WriteRune(smallest)
	}
	return folded.String()
}
