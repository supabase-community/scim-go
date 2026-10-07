// Package scimtest checks a SCIM implementation against the JSON examples of RFC 7643, Section 8.
package scimtest

import (
	"embed"
	"path"
)

//go:embed testdata
var files embed.FS

type TB interface {
	Helper()
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
}

func Golden(t TB, name string) []byte {
	t.Helper()

	data, err := files.ReadFile(path.Join("testdata", name))
	if err != nil {
		t.Fatalf("scimtest: %v", err)
		return nil
	}
	return data
}
