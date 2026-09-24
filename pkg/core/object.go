package core

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/supabase-community/scim-go/internal/decode"
)

// Object is a JSON object whose attribute names are case insensitive, per RFC 7643, Section 2.1.
type Object map[string]any

func NewObject(v any) (Object, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return decode.JSON[Object](bytes.NewReader(raw))
}

func (o Object) Get(name string) any {
	return o[o.key(name)]
}

func (o Object) Has(name string) bool {
	_, ok := o[o.key(name)]
	return ok
}

func (o Object) Set(name string, value any) {
	o[o.key(name)] = value
}

func (o Object) Remove(name string) {
	delete(o, o.key(name))
}

func (o Object) key(name string) string {
	if _, ok := o[name]; ok {
		return name
	}
	match := ""
	for candidate := range o {
		if strings.EqualFold(candidate, name) && (match == "" || candidate < match) {
			match = candidate
		}
	}
	if match == "" {
		return name
	}
	return match
}
