package server

import (
	"time"

	"github.com/supabase-community/scim-go/internal/etag"
)

// WeakETag is a weak entity tag for a resource last modified at t, per RFC 7644, Section 3.14. It keeps microseconds, the precision of a PostgreSQL timestamptz.
func WeakETag(t time.Time) string {
	return etag.Weak(t)
}

// ParseWeakETag returns the modification time that WeakETag encoded in version.
func ParseWeakETag(version string) (time.Time, bool) {
	return etag.Parse(version)
}
