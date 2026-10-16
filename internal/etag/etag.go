package etag

import (
	"strconv"
	"strings"
	"time"
)

// Weak is a weak entity tag for a resource last modified at t, per RFC 7644, Section 3.14. It keeps microseconds, the precision of a PostgreSQL timestamptz.
func Weak(t time.Time) string {
	return `W/"` + strconv.FormatInt(t.UnixMicro(), 10) + `"`
}

// Parse reads the modification time from a weak entity tag, per RFC 7232, Section 2.3.
func Parse(version string) (time.Time, bool) {
	digits, found := strings.CutPrefix(version, `W/"`)
	digits, closed := strings.CutSuffix(digits, `"`)
	micros, err := strconv.ParseInt(digits, 10, 64)
	if !found || !closed || err != nil || micros < 0 {
		return time.Time{}, false
	}
	return time.UnixMicro(micros).UTC(), true
}
