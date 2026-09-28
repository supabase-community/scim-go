package value

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/supabase-community/scim-go/pkg/core"
)

// Set indexes elements by a fold-aware bucket key so Contains, used to dedupe an "add" per RFC 7644 Section 3.5.2.1, checks a candidate against its bucket instead of every element.
type Set[E any] struct {
	attribute *core.Attribute
	buckets   map[string][]E
}

// NewSet buckets elements for repeated Contains checks against the same list.
func NewSet[E any](attribute *core.Attribute, elements []E) *Set[E] {
	buckets := make(map[string][]E, len(elements))
	for _, element := range elements {
		key := bucketKey(attribute, element)
		buckets[key] = append(buckets[key], element)
	}
	return &Set[E]{attribute: attribute, buckets: buckets}
}

// Contains reports whether want equals any indexed element, per RFC 7643, Section 2.3.
func (s *Set[E]) Contains(want any) bool {
	bucket := s.buckets[bucketKey(s.attribute, want)]
	return slices.ContainsFunc(bucket, func(element E) bool { return Equal(s.attribute, element, want) })
}

// bucketKey groups values Equal would treat alike: Fold decides string case for scalars, an instant
// decides a time.Time regardless of its zone, and a JSON encoding decides a complex element.
func bucketKey(attribute *core.Attribute, v any) string {
	switch value := v.(type) {
	case map[string]any:
		raw, _ := json.Marshal(value)
		return string(raw)
	case time.Time:
		return value.UTC().Format(time.RFC3339Nano)
	}
	raw, _ := json.Marshal(Fold(attribute, v))
	return string(raw)
}
