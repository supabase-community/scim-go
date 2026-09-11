package protocol

// PatchOp is the kind of modification in a PATCH operation, per RFC 7644, Section 3.5.2.
type PatchOp string

const (
	PatchOpAdd     PatchOp = "add"
	PatchOpRemove  PatchOp = "remove"
	PatchOpReplace PatchOp = "replace"
)
