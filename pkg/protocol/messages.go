package protocol

import (
	"github.com/supabase-community/scim-go/pkg/core"
	"github.com/supabase-community/scim-go/pkg/scimerrors"
)

const (
	messagesRoot = "urn:ietf:params:scim:api:messages:2.0"

	SchemaBulkRequest   core.SchemaURI = messagesRoot + ":BulkRequest"
	SchemaBulkResponse  core.SchemaURI = messagesRoot + ":BulkResponse"
	SchemaError                        = scimerrors.SchemaError
	SchemaListResponse  core.SchemaURI = messagesRoot + ":ListResponse"
	SchemaPatchOp       core.SchemaURI = messagesRoot + ":PatchOp"
	SchemaSearchRequest core.SchemaURI = messagesRoot + ":SearchRequest"
)
