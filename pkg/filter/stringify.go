package filter

import (
	"encoding/json"
	"fmt"
	"strconv"
)

type Stringify struct{}

func (Stringify) VisitAnd(left, right string) (string, error) {
	return fmt.Sprintf("(%s and %s)", left, right), nil
}

func (Stringify) VisitOr(left, right string) (string, error) {
	return fmt.Sprintf("(%s or %s)", left, right), nil
}

func (Stringify) VisitNot(operand string) (string, error) {
	return fmt.Sprintf("not (%s)", operand), nil
}

func (Stringify) VisitEquals(attribute AttrPath, value any) (string, error) {
	return compareString(attribute, "eq", value), nil
}

func (Stringify) VisitNotEquals(attribute AttrPath, value any) (string, error) {
	return compareString(attribute, "ne", value), nil
}

func (Stringify) VisitContains(attribute AttrPath, value any) (string, error) {
	return compareString(attribute, "co", value), nil
}

func (Stringify) VisitStartsWith(attribute AttrPath, value any) (string, error) {
	return compareString(attribute, "sw", value), nil
}

func (Stringify) VisitEndsWith(attribute AttrPath, value any) (string, error) {
	return compareString(attribute, "ew", value), nil
}

func (Stringify) VisitGreaterThan(attribute AttrPath, value any) (string, error) {
	return compareString(attribute, "gt", value), nil
}

func (Stringify) VisitGreaterThanEquals(attribute AttrPath, value any) (string, error) {
	return compareString(attribute, "ge", value), nil
}

func (Stringify) VisitLessThan(attribute AttrPath, value any) (string, error) {
	return compareString(attribute, "lt", value), nil
}

func (Stringify) VisitLessThanEquals(attribute AttrPath, value any) (string, error) {
	return compareString(attribute, "le", value), nil
}

func (Stringify) VisitPresence(attribute AttrPath) (string, error) {
	return fmt.Sprintf("%s pr", attribute), nil
}

func (Stringify) VisitValuePath(path AttrPath, subAttribute string, valueFilter func() (string, error)) (string, error) {
	vf, err := valueFilter()
	if err != nil {
		return "", err
	}
	if subAttribute == "" {
		return fmt.Sprintf("%s[%s]", path, vf), nil
	}
	return fmt.Sprintf("%s[%s].%s", path, vf, subAttribute), nil
}

func compareString(attribute AttrPath, operator string, value any) string {
	return fmt.Sprintf("%s %s %s", attribute, operator, stringifyValue(value))
}

func stringifyValue(value any) string {
	switch v := value.(type) {
	case string:
		b, _ := json.Marshal(v)
		return string(b)
	case bool:
		return strconv.FormatBool(v)
	case json.Number:
		return v.String()
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", v)
	}
}
