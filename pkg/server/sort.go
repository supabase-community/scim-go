package server

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

type sortEntry[T any] struct {
	item T
	key  string
}

func sortItems[T any](items []T, sortBy string, descending bool) {
	if sortBy == "" {
		return
	}
	segments := strings.Split(strings.ToLower(sortBy), ".")

	entries := make([]sortEntry[T], len(items))
	for i, item := range items {
		entries[i] = sortEntry[T]{item: item, key: sortKey(item, segments)}
	}
	slices.SortFunc(entries, func(a, b sortEntry[T]) int {
		c := strings.Compare(a.key, b.key)
		if descending {
			return -c
		}
		return c
	})
	for i, entry := range entries {
		items[i] = entry.item
	}
}

func sortKey[T any](item T, segments []string) string {
	raw, err := json.Marshal(item)
	if err != nil {
		return ""
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return ""
	}
	values := lookup(doc, segments)
	if len(values) == 0 {
		return ""
	}
	return fmt.Sprint(values[0])
}
