package scimtest

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func AssertJSON(t TB, name string, value any) bool {
	t.Helper()

	want, err := decode(Golden(t, name))
	if err != nil {
		t.Fatalf("scimtest: reading %s: %v", name, err)
		return false
	}

	got, err := remarshal(value)
	if err != nil {
		t.Fatalf("scimtest: encoding %T: %v", value, err)
		return false
	}

	differences := diff("", want, got)
	if len(differences) == 0 {
		return true
	}

	t.Errorf("scimtest: %T does not match %s:\n%s", value, name, join(differences))
	return false
}

// RoundTripDiff decodes the named golden file into value and lists the paths
// at which encoding value again differs from the golden file, so value must
// be a pointer.
func RoundTripDiff(t TB, name string, value any) []string {
	t.Helper()

	golden := Golden(t, name)
	if err := json.Unmarshal(golden, value); err != nil {
		t.Fatalf("scimtest: decoding %s into %T: %v", name, value, err)
		return nil
	}

	want, err := decode(golden)
	if err != nil {
		t.Fatalf("scimtest: reading %s: %v", name, err)
		return nil
	}

	got, err := remarshal(value)
	if err != nil {
		t.Fatalf("scimtest: encoding %T: %v", value, err)
		return nil
	}

	paths := diff("", want, got)
	sort.Strings(paths)
	return paths
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

func diffObject(path string, want, got map[string]any) []string {
	var paths []string

	for _, key := range sorted(want) {
		actual, present := lookup(got, key)
		if !present {
			paths = append(paths, join1(path, key))
			continue
		}
		paths = append(paths, diff(join1(path, key), want[key], actual)...)
	}

	for _, key := range sorted(got) {
		if _, present := lookup(want, key); !present {
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

// lookup matches an attribute name case insensitively, per RFC 7643, Section 2.1.
func lookup(object map[string]any, key string) (any, bool) {
	if value, ok := object[key]; ok {
		return value, true
	}

	for name, value := range object {
		if strings.EqualFold(name, key) {
			return value, true
		}
	}
	return nil, false
}

func sorted(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func join1(path, key string) string {
	if path == "" {
		return key
	}
	return path + "." + key
}

func join(paths []string) string {
	var out strings.Builder
	for _, p := range paths {
		out.WriteString("  ")
		out.WriteString(p)
		out.WriteString("\n")
	}
	return out.String()
}
