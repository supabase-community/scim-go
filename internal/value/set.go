package value

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/supabase-community/scim-go/pkg/core"
)

// Set buckets elements so Contains, per RFC 7643, Section 2.3, checks a candidate against its bucket instead of every element.
type Set struct {
	attribute *core.Attribute
	buckets   map[string][]any
}

func NewSet(attribute *core.Attribute, elements []any) *Set {
	buckets := make(map[string][]any, len(elements))
	for _, element := range elements {
		key := bucketKey(attribute, element)
		buckets[key] = append(buckets[key], element)
	}
	return &Set{attribute: attribute, buckets: buckets}
}

func (s *Set) Contains(want any) bool {
	bucket := s.buckets[bucketKey(s.attribute, want)]
	return slices.ContainsFunc(bucket, func(element any) bool { return Equal(s.attribute, element, want) })
}

func bucketKey(attribute *core.Attribute, v any) string {
	switch value := v.(type) {
	case map[string]any:
		raw, _ := json.Marshal(value)
		return string(raw)
	case time.Time:
		return value.UTC().Format(time.RFC3339Nano)
	case float64:
		if value == 0 {
			value = 0
		}
		raw, _ := json.Marshal(value)
		return string(raw)
	}
	raw, _ := json.Marshal(Fold(attribute, v))
	return string(raw)
}
