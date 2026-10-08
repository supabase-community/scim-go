package core

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"time"
)

// AttributeType is the data type of an attribute, per RFC 7643 Section 2.3.
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
		return text(value)
	case TypeBinary:
		s, ok := value.(string)
		if !ok || !isBase64(s) {
			return nil, false
		}
		return value, true
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
		return dateTime(s)
	}
	return nil, false
}

// RFC 7643 Section 2.3.5: a DateTime is an xsd:dateTime, whose time zone offset is optional.
func dateTime(s string) (any, bool) {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05.999999999", s)
	}
	return t, err == nil
}

func text(value any) (any, bool) {
	if _, ok := value.(string); !ok {
		return "", false
	}
	return value, true
}

func decimal(value any) (any, bool) {
	switch n := value.(type) {
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	case float64:
		return value, true
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
		return value, true
	case int:
		return int64(n), true
	case float64:
		return int64(n), n == math.Trunc(n)
	}
	return nil, false
}

// RFC 7643 Section 2.3.6: base64 per RFC 4648 Section 4, trailing padding MAY be omitted.
func isBase64(s string) bool {
	if _, err := base64.StdEncoding.DecodeString(s); err == nil {
		return true
	}
	_, err := base64.RawStdEncoding.DecodeString(s)
	return err == nil
}
