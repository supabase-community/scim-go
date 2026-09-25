package core

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"time"
)

// AttributeType is the data type of an attribute, per RFC 7643, Section 7.
type AttributeType string

const (
	TypeString    AttributeType = "string"
	TypeBoolean   AttributeType = "boolean"
	TypeDecimal   AttributeType = "decimal"
	TypeInteger   AttributeType = "integer"
	TypeDateTime  AttributeType = "dateTime"
	TypeBinary    AttributeType = "binary"
	TypeReference AttributeType = "reference"
	TypeComplex   AttributeType = "complex"
)

func (a AttributeType) coerce(value any) (any, bool) {
	switch a {
	case TypeString, TypeReference:
		s, ok := value.(string)
		return s, ok
	case TypeBinary:
		s, ok := value.(string)
		if !ok {
			return nil, false
		}
		// RFC 7643 Section 2.3.6: binary values are base64 encoded.
		if _, err := base64.StdEncoding.DecodeString(s); err != nil {
			return nil, false
		}
		return s, true
	case TypeBoolean:
		b, ok := value.(bool)
		return b, ok
	case TypeDecimal:
		return decimal(value)
	case TypeInteger:
		return integer(value)
	case TypeDateTime:
		s, ok := value.(string)
		if !ok {
			return nil, false
		}
		t, err := time.Parse(time.RFC3339, s)
		return t, err == nil
	}
	return nil, false
}

func decimal(value any) (any, bool) {
	switch n := value.(type) {
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	}
	return nil, false
}

func integer(value any) (any, bool) {
	switch n := value.(type) {
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64:
		return int64(n), n == math.Trunc(n)
	}
	return nil, false
}
