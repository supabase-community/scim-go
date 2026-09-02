package scim

import "strings"

// BasePath is the mount point of the SCIM service, per RFC 7644, Section 3.2.
const BasePath = "/scim/v2"

func Join(base, segment string) string {
	return strings.TrimSuffix(base, "/") + "/" + strings.TrimPrefix(segment, "/")
}
