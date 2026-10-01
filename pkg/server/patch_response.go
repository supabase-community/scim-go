package server

import (
	"net/http"
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/filter"
	"github.com/supabase-community/scim-go/pkg/patch"
)

// noContentEligible reports whether a PATCH response may omit the body, per RFC 7644 Section 3.5.2.
func noContentEligible(r *http.Request, schemas core.Schemas, ops []patch.Operation) bool {
	if schemas.Base().ID != core.SchemaGroup {
		return false
	}
	if query := r.URL.Query(); query.Has("attributes") || query.Has("excludedAttributes") {
		return false
	}
	for _, op := range ops {
		attribute, ok := attributeOf(op)
		if !ok || !strings.EqualFold(attribute, "members") {
			return false
		}
	}
	return true
}

func attributeOf(op patch.Operation) (string, bool) {
	path, err := filter.NewPath(op.Path)
	if err != nil || path.URI != "" || path.SubAttribute != "" {
		return "", false
	}
	switch patch.Op(strings.ToLower(string(op.Op))) {
	case patch.OpAdd:
		return path.Name, path.ValueFilter == nil
	case patch.OpRemove:
		return path.Name, isValueEquality(path.ValueFilter)
	default:
		return "", false
	}
}

func isValueEquality(node *filter.Node) bool {
	if node == nil || node.Not() || filter.Operator(node.Operator()) != filter.OpEquals {
		return false
	}
	attr := node.AttrPath()
	return attr.URI == "" && attr.SubAttribute == "" && strings.EqualFold(attr.Name, "value") && node.Value() != nil
}
