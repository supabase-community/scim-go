package filter

import (
	"encoding/json"
	"fmt"
	"strconv"
)

type StringifyVisitor struct{}

func (StringifyVisitor) VisitAnd(left, right string) (string, error) {
	return fmt.Sprintf("(%s and %s)", left, right), nil
}

func (StringifyVisitor) VisitOr(left, right string) (string, error) {
	return fmt.Sprintf("(%s or %s)", left, right), nil
}

func (StringifyVisitor) VisitNot(operand string) (string, error) {
	return fmt.Sprintf("not (%s)", operand), nil
}

func (StringifyVisitor) VisitEquals(attribute AttrPath, value any) (string, error) {
	return compareString(attribute, "eq", value), nil
}

func (StringifyVisitor) VisitNotEquals(attribute AttrPath, value any) (string, error) {
	return compareString(attribute, "ne", value), nil
}

func (StringifyVisitor) VisitContains(attribute AttrPath, value any) (string, error) {
	return compareString(attribute, "co", value), nil
}

func (StringifyVisitor) VisitStartsWith(attribute AttrPath, value any) (string, error) {
	return compareString(attribute, "sw", value), nil
}

func (StringifyVisitor) VisitEndsWith(attribute AttrPath, value any) (string, error) {
	return compareString(attribute, "ew", value), nil
}

func (StringifyVisitor) VisitGreaterThan(attribute AttrPath, value any) (string, error) {
	return compareString(attribute, "gt", value), nil
}

func (StringifyVisitor) VisitGreaterThanEquals(attribute AttrPath, value any) (string, error) {
	return compareString(attribute, "ge", value), nil
}

func (StringifyVisitor) VisitLessThan(attribute AttrPath, value any) (string, error) {
	return compareString(attribute, "lt", value), nil
}

func (StringifyVisitor) VisitLessThanEquals(attribute AttrPath, value any) (string, error) {
	return compareString(attribute, "le", value), nil
}

func (StringifyVisitor) VisitPresence(attribute AttrPath) (string, error) {
	return fmt.Sprintf("%s pr", attribute), nil
}

func (StringifyVisitor) VisitValuePath(path AttrPath, subAttribute string, valueFilter func() (string, error)) (string, error) {
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
	case float64:
		return strconv.FormatFloat(v, 'g', -1, 64)
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", v)
	}
}
