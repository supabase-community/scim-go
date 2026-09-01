// Package scimtest checks a SCIM implementation against the JSON examples of RFC 7643, Section 8.
package scimtest

import (
	"embed"
	"io/fs"
	"path"
	"sort"
	"strings"
)

//go:embed testdata
var files embed.FS

const root = "testdata"

type TB interface {
	Helper()
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
}

func Load(name string) ([]byte, error) {
	return files.ReadFile(path.Join(root, name))
}

func Golden(t TB, name string) []byte {
	t.Helper()

	data, err := Load(name)
	if err != nil {
		t.Fatalf("scimtest: %v", err)
		return nil
	}
	return data
}

func Names() []string {
	var names []string

	_ = fs.WalkDir(files, root, func(p string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		if name, found := strings.CutPrefix(p, root+"/"); found {
			names = append(names, name)
		}
		return nil
	})

	sort.Strings(names)
	return names
}
