package server

import (
	"strconv"
	"strings"
	"time"
)

// WeakETag is a weak entity tag for a resource last modified at t, per RFC 7644, Section 3.14. It keeps microseconds, the precision of a PostgreSQL timestamptz.
func WeakETag(t time.Time) string {
	return `W/"` + strconv.FormatInt(t.UnixMicro(), 10) + `"`
}

// ParseWeakETag returns the modification time that WeakETag encoded in version.
func ParseWeakETag(version string) (time.Time, bool) {
	digits, found := strings.CutPrefix(version, `W/"`)
	digits, closed := strings.CutSuffix(digits, `"`)
	micros, err := strconv.ParseInt(digits, 10, 64)
	if !found || !closed || err != nil || micros < 0 {
		return time.Time{}, false
	}
	return time.UnixMicro(micros).UTC(), true
}
