package value

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/supabase-community/scim-go/pkg/core"
)

// Identity returns the key of element within a multi-valued attribute; RFC 7643 Section 2.4: the same "value" MAY repeat with a different "type".
func Identity(attribute *core.Attribute, element core.Object) string {
	sub := attribute.SubAttribute("value")
	if sub == nil {
		return compositeIdentity(attribute, element)
	}
	key, ok := Key(element)
	if !ok {
		return ""
	}
	folded := Fold(sub, key)
	kind := attribute.SubAttribute("type")
	if kind == nil || kind.Mutability != core.MutabilityReadWrite {
		return folded.(string)
	}
	typ := Fold(kind, element.Get(kind.Name))
	if IsUnassigned(typ) {
		typ = nil
	}
	return identityKey([]any{folded, typ})
}

func compositeIdentity(attribute *core.Attribute, element core.Object) string {
	folded := make([]any, 0, len(attribute.SubAttributes))
	for _, sub := range attribute.SubAttributes {
		if sub.Mutability == core.MutabilityReadWrite && !Hidden(nil, sub) && !strings.EqualFold(sub.Name, "primary") {
			folded = append(folded, canonical(sub, element.Get(sub.Name)))
		}
	}
	if len(folded) == 0 {
		return ""
	}
	return identityKey(folded)
}

func identityKey(values []any) string {
	raw := make([]byte, 0, 64)
	for _, v := range values {
		switch item := v.(type) {
		case string:
			raw = strconv.AppendQuote(raw, item)
		case nil:
			raw = append(raw, "null"...)
		case bool:
			raw = strconv.AppendBool(raw, item)
		case json.Number:
			raw = append(raw, item...)
		case float64:
			raw = strconv.AppendFloat(raw, normalizeZero(item), 'g', -1, 64)
		case int64:
			raw = strconv.AppendInt(raw, item, 10)
		case time.Time:
			raw = item.UTC().AppendFormat(raw, time.RFC3339Nano)
		default:
			encoded, _ := json.Marshal(item)
			raw = append(raw, encoded...)
		}
		raw = append(raw, ',')
	}
	return string(raw)
}
