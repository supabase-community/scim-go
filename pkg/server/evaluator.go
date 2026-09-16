package server

import (
	"encoding/json"
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

type boolEvaluator struct {
	current map[string]any
}

func matchesFilter[T any](schemas []*core.Schema, text string, item T) (bool, error) {
	doc, err := toDoc(item)
	if err != nil {
		return false, err
	}
	return protocol.Filter[bool](schemas, text, &boolEvaluator{current: doc})
}

func toDoc[T any](item T) (map[string]any, error) {
	raw, err := json.Marshal(item)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func (e *boolEvaluator) Compare(attr *core.Attribute, key string, op filter.Operator, value any) (bool, error) {
	for _, candidate := range lookup(e.current, strings.Split(key, ".")) {
		if compareOne(attr, op, candidate, value) {
			return true, nil
		}
	}
	return false, nil
}

func (e *boolEvaluator) Present(_ *core.Attribute, key string) (bool, error) {
	for _, candidate := range lookup(e.current, strings.Split(key, ".")) {
		if !isEmpty(candidate) {
			return true, nil
		}
	}
	return false, nil
}

func (e *boolEvaluator) And(left, right bool) (bool, error) { return left && right, nil }
func (e *boolEvaluator) Or(left, right bool) (bool, error)  { return left || right, nil }
func (e *boolEvaluator) Not(operand bool) (bool, error)     { return !operand, nil }

func (e *boolEvaluator) ValuePath(_ *core.Attribute, key string, valueFilter func() (bool, error)) (bool, error) {
	for _, candidate := range lookup(e.current, strings.Split(key, ".")) {
		elem, ok := candidate.(map[string]any)
		if !ok {
			continue
		}
		saved := e.current
		e.current = elem
		matched, err := valueFilter()
		e.current = saved
		if err != nil {
			return false, err
		}
		if matched {
			return true, nil
		}
	}
	return false, nil
}
