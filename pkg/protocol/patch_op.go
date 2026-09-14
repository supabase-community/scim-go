package protocol

import "github.com/supabase-community/scim-go/pkg/patch"

// PatchOp moved to pkg/patch; this alias preserves the protocol surface.
type PatchOp = patch.Op

const (
	PatchOpAdd     = patch.OpAdd
	PatchOpRemove  = patch.OpRemove
	PatchOpReplace = patch.OpReplace
)
