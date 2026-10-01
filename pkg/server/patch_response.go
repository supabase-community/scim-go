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
		shape, ok := parseAttributeOp(op)
		if !ok || !strings.EqualFold(shape.attribute, "members") {
			return false
		}
	}
	return true
}

type attributeOp struct {
	attribute string
	add       bool
	literal   any
}

func parseAttributeOp(op patch.Operation) (attributeOp, bool) {
	path, err := filter.NewPath(op.Path)
	if err != nil || path.URI != "" || path.SubAttribute != "" {
		return attributeOp{}, false
	}
	switch patch.Op(strings.ToLower(string(op.Op))) {
	case patch.OpAdd:
		if path.ValueFilter != nil {
			return attributeOp{}, false
		}
		return attributeOp{attribute: path.Name, add: true}, true
	case patch.OpRemove:
		literal, ok := bareValueLiteral(path.ValueFilter)
		if !ok {
			return attributeOp{}, false
		}
		return attributeOp{attribute: path.Name, literal: literal}, true
	default:
		return attributeOp{}, false
	}
}

func bareValueLiteral(node *filter.Node) (any, bool) {
	if node == nil || node.Not() {
		return nil, false
	}
	if filter.Operator(node.Operator()) != filter.OpEquals {
		return nil, false
	}
	attr := node.AttrPath()
	if attr.URI != "" || attr.SubAttribute != "" || !strings.EqualFold(attr.Name, "value") {
		return nil, false
	}
	if value := node.Value(); value != nil {
		return value, true
	}
	return nil, false
}
