package value

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/supabase-community/scim-go/pkg/core"
)

type Set struct {
	attribute *core.Attribute
	buckets   map[string][]any
}

func NewSet(attribute *core.Attribute, elements []any) *Set {
	s := &Set{attribute: attribute, buckets: make(map[string][]any, len(elements))}
	s.Add(elements...)
	return s
}

func (s *Set) Add(elements ...any) {
	for _, element := range elements {
		key := bucketKey(s.attribute, element)
		s.buckets[key] = append(s.buckets[key], element)
	}
}

func (s *Set) Contains(want any) bool {
	bucket := s.buckets[bucketKey(s.attribute, want)]
	return slices.ContainsFunc(bucket, func(element any) bool { return Equal(s.attribute, element, want) })
}

func bucketKey(attribute *core.Attribute, v any) string {
	v = typed(attribute, v)
	switch value := v.(type) {
	case map[string]any, []any:
		raw, _ := json.Marshal(normalizeZeros(value))
		return string(raw)
	case time.Time:
		return value.UTC().Format(time.RFC3339Nano)
	case float64:
		raw, _ := json.Marshal(normalizeZero(value))
		return string(raw)
	}
	raw, _ := json.Marshal(Fold(attribute, v))
	return string(raw)
}

func normalizeZero(f float64) float64 {
	if f == 0 {
		return 0
	}
	return f
}

func normalizeZeros(v any) any {
	return transform(v, func(leaf any) any {
		if f, ok := leaf.(float64); ok {
			return normalizeZero(f)
		}
		return leaf
	})
}
