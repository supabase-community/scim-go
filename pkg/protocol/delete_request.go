package protocol

// DeleteRequest identifies the resource a DELETE removes, per RFC 7644, Section 3.6.
type DeleteRequest struct {
	ID      string
	Version string
}
