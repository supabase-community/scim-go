package scimtest

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/patch"
	"github.com/supabase-community/scim-go/pkg/protocol"
)

func filterBy(expression string) url.Values {
	return url.Values{"filter": {expression}}
}

func byID(ids ...string) url.Values {
	return filterBy(idFilter(ids...))
}

func idFilter(ids ...string) string {
	terms := make([]string, 0, len(ids))
	for _, id := range ids {
		terms = append(terms, "id eq "+quoted(id))
	}
	return strings.Join(terms, " or ")
}

func quoted(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func ops(operations ...patch.Operation) protocol.PatchRequest {
	return protocol.PatchRequest{Schemas: []core.SchemaURI{protocol.SchemaPatchOp}, Operations: operations}
}

func op(kind patch.Op, path string, value any) patch.Operation {
	operation := patch.Operation{Op: kind, Path: path}
	if value != nil {
		operation.Value, _ = json.Marshal(value)
	}
	return operation
}
