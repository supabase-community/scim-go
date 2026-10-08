package value

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/supabase-community/scim-go/pkg/core"
)

type Set struct {
	attribute *core.Attribute
	objects   map[string]core.Object
	buckets   map[string][]any
}

func NewSet(attribute *core.Attribute, elements []any) *Set {
	s := &Set{attribute: attribute}
	if KeyedByIdentity(attribute) {
		s.objects = make(map[string]core.Object, len(elements))
	} else {
		s.buckets = make(map[string][]any, len(elements))
	}
	s.Add(elements...)
	return s
}

func KeyedByIdentity(attribute *core.Attribute) bool {
	return len(attribute.SubAttributes) > 0
}

func (s *Set) Add(elements ...any) {
	for _, element := range elements {
		if s.objects != nil {
			s.addObject(AsObject(element))
			continue
		}
		key := bucketKey(s.attribute, element)
		s.buckets[key] = append(s.buckets[key], element)
	}
}

func (s *Set) Contains(want any) bool {
	if s.objects != nil {
		return s.Find(AsObject(want)) != nil
	}
	bucket := s.buckets[bucketKey(s.attribute, want)]
	return slices.ContainsFunc(bucket, func(element any) bool { return Equal(s.attribute, element, want) })
}

func (s *Set) Find(element core.Object) core.Object {
	return s.objects[Identity(s.attribute, element)]
}

func (s *Set) addObject(object core.Object) {
	if object == nil {
		return
	}
	if id := Identity(s.attribute, object); id != "" && s.objects[id] == nil {
		s.objects[id] = object
	}
}

func bucketKey(attribute *core.Attribute, v any) string {
	v = typed(attribute, v)
	switch value := v.(type) {
	case map[string]any, core.Object, []any:
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
