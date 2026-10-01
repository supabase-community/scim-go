package server

import (
	"encoding/json"
	"slices"
	"strings"

	"github.com/supabase-community/scim-go/internal/value"
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
)

type delta struct {
	attribute string
	added     []core.Object
	removed   []string
}

// eligibleDelta extracts an add/remove-only delta, the only PATCH shape RFC 7643 Section 4.2 immutability can't block.
func eligibleDelta(schemas core.Schemas, ops []patch.Operation) (delta, bool) {
	var d delta
	for _, op := range ops {
		if !accumulateOp(&d, op) {
			return delta{}, false
		}
	}
	attribute, ok := resolveDeltaAttribute(schemas, d)
	if !ok || !everyAddedHasValue(d.added) || !validAddedElements(attribute, d.added) || identityOverlap(attribute, d) {
		return delta{}, false
	}
	return d, true
}

func accumulateOp(d *delta, op patch.Operation) bool {
	shape, ok := parseAttributeOp(op)
	if !ok {
		return false
	}
	if d.attribute == "" {
		d.attribute = shape.attribute
	} else if !strings.EqualFold(d.attribute, shape.attribute) {
		return false
	}
	if !shape.add {
		literal, ok := shape.literal.(string)
		if !ok {
			return false
		}
		d.removed = append(d.removed, literal)
		return true
	}
	added, ok := addedElements(op.Value)
	if !ok {
		return false
	}
	d.added = append(d.added, added...)
	return true
}

func resolveDeltaAttribute(schemas core.Schemas, d delta) (*core.Attribute, bool) {
	if d.attribute == "" {
		return nil, false
	}
	attribute, ok := schemas.Resolve("", d.attribute, "")
	if !ok || !attribute.MultiValued || attribute.SubAttribute("value") == nil {
		return nil, false
	}
	return attribute, true
}

func addedElements(raw json.RawMessage) ([]core.Object, bool) {
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, false
	}
	switch v := decoded.(type) {
	case []any:
		elements := make([]core.Object, 0, len(v))
		for _, item := range v {
			object, ok := item.(map[string]any)
			if !ok {
				return nil, false
			}
			elements = append(elements, core.Object(object))
		}
		return elements, true
	case map[string]any:
		return []core.Object{core.Object(v)}, true
	default:
		return nil, false
	}
}

func everyAddedHasValue(added []core.Object) bool {
	for _, object := range added {
		if value.IsUnassigned(object.Get("value")) {
			return false
		}
	}
	return true
}

// validAddedElements checks added against RFC 7643 Section 2.2 characteristics, the same way characteristics does.
func validAddedElements(attribute *core.Attribute, added []core.Object) bool {
	if hasPrimarySubAttribute(attribute) {
		return false
	}
	parent := field{Attribute: attribute, raw: func(core.Object) any { return addedAsElements(added) }}
	for _, sub := range attribute.SubAttributes {
		if !coercibleAdded(sub, added) {
			return false
		}
		if f := parent.sub(sub); isConstrained(f) && conforms(f, core.Object{}) != nil {
			return false
		}
	}
	return true
}

func addedAsElements(added []core.Object) []any {
	elements := make([]any, len(added))
	for i, element := range added {
		elements[i] = map[string]any(element)
	}
	return elements
}

func coercibleAdded(sub *core.Attribute, added []core.Object) bool {
	for _, element := range added {
		raw := element.Get(sub.Name)
		if value.IsUnassigned(raw) {
			continue
		}
		if _, ok := sub.Coerce(raw); !ok {
			return false
		}
	}
	return true
}

func hasPrimarySubAttribute(attribute *core.Attribute) bool {
	return slices.ContainsFunc(attribute.SubAttributes, isPrimaryField)
}

func identityOverlap(attribute *core.Attribute, d delta) bool {
	removed := make(map[string]bool, len(d.removed))
	for _, literal := range d.removed {
		removed[value.Identity(attribute, core.Object{"value": literal})] = true
	}
	for _, object := range d.added {
		if removed[value.Identity(attribute, object)] {
			return true
		}
	}
	return false
}
