package memory

import "github.com/supabase-community/scim-go/pkg/core"

type uniqueIndex map[indexKey]string

// RFC 7643 Section 7: a unique value may be held by only one resource.
func (u uniqueIndex) conflict(id string, keys []indexKey) *core.Attribute {
	for _, key := range keys {
		if holder, ok := u[key]; ok && holder != id {
			return key.attribute
		}
	}
	return nil
}

func (u uniqueIndex) add(keys []indexKey, id string) {
	for _, key := range keys {
		u[key] = id
	}
}

func (u uniqueIndex) remove(keys []indexKey) {
	for _, key := range keys {
		delete(u, key)
	}
}
