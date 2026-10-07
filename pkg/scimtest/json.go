package scimtest

import (
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/supabase-community/scim-go/pkg/core"
)

func AssertJSON(t TB, name string, value any, ignore ...string) bool {
	t.Helper()

	paths, ok := goldenDiff(t, name, value)
	if !ok {
		return false
	}

	differences := slices.DeleteFunc(paths, func(path string) bool { return slices.Contains(ignore, path) })
	if len(differences) == 0 {
		return true
	}

	t.Errorf("scimtest: %T does not match %s:\n%s", value, name, "  "+strings.Join(differences, "\n  ")+"\n")
	return false
}

func RoundTripDiff(t TB, name string, value any) []string {
	t.Helper()

	if err := json.Unmarshal(Golden(t, name), value); err != nil {
		t.Fatalf("scimtest: decoding %s into %T: %v", name, value, err)
		return nil
	}

	paths, ok := goldenDiff(t, name, value)
	if !ok {
		return nil
	}

	slices.Sort(paths)
	return paths
}

func goldenDiff(t TB, name string, value any) ([]string, bool) {
	t.Helper()

	want, err := decode(Golden(t, name))
	if err != nil {
		t.Fatalf("scimtest: reading %s: %v", name, err)
		return nil, false
	}

	got, err := remarshal(value)
	if err != nil {
		t.Fatalf("scimtest: encoding %T: %v", value, err)
		return nil, false
	}

	return diff("", want, got), true
}

func decode(data []byte) (any, error) {
	var value any
	err := json.Unmarshal(data, &value)
	return value, err
}

func remarshal(value any) (any, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return decode(data)
}

func diff(path string, want, got any) []string {
	switch expected := want.(type) {
	case map[string]any:
		actual, ok := got.(map[string]any)
		if !ok {
			return []string{path}
		}
		return diffObject(path, expected, actual)

	case []any:
		actual, ok := got.([]any)
		if !ok {
			return []string{path}
		}
		return diffArray(path, expected, actual)

	default:
		if fmt.Sprint(want) != fmt.Sprint(got) {
			return []string{path}
		}
		return nil
	}
}

func diffObject(path string, want, got core.Object) []string {
	var paths []string

	for _, key := range slices.Sorted(maps.Keys(want)) {
		if !got.Has(key) {
			paths = append(paths, join1(path, key))
			continue
		}
		paths = append(paths, diff(join1(path, key), want[key], got.Get(key))...)
	}

	for _, key := range slices.Sorted(maps.Keys(got)) {
		if !want.Has(key) {
			paths = append(paths, join1(path, key))
		}
	}

	return paths
}

func diffArray(path string, want, got []any) []string {
	if len(want) != len(got) {
		return []string{path}
	}

	var paths []string
	for i := range want {
		paths = append(paths, diff(path+"["+strconv.Itoa(i)+"]", want[i], got[i])...)
	}
	return paths
}

func join1(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}
