package patch

import (
	"reflect"
	"slices"

	"github.com/supabase-community/scim-go/pkg/core"
)

type primaries map[uintptr]bool

func primariesOf(root core.Object) primaries {
	found := primaries{}
	for _, list := range lists(root) {
		for _, member := range membersOf(list) {
			if isPrimary(member) {
				found[pointer(member)] = true
			}
		}
	}
	return found
}

// RFC 7644 Section 3.5.2: setting "primary" to true sets it to false for every other value of the attribute.
func (before primaries) demote(root core.Object) {
	for _, list := range lists(root) {
		members := membersOf(list)
		if !slices.ContainsFunc(members, before.promoted) {
			continue
		}
		for _, member := range members {
			if before.has(member) {
				core.Object(member).Set("primary", false)
			}
		}
	}
}

func (before primaries) promoted(member map[string]any) bool {
	return isPrimary(member) && !before.has(member)
}

func (before primaries) has(member map[string]any) bool {
	return before[pointer(member)]
}

func pointer(member map[string]any) uintptr {
	return reflect.ValueOf(member).Pointer()
}

func lists(object map[string]any) [][]any {
	var found [][]any
	for _, value := range object {
		switch typed := value.(type) {
		case []any:
			found = append(found, typed)
		case map[string]any:
			found = append(found, lists(typed)...)
		}
	}
	return found
}

func membersOf(list []any) []map[string]any {
	members := make([]map[string]any, 0, len(list))
	for _, element := range list {
		if member, ok := element.(map[string]any); ok {
			members = append(members, member)
		}
	}
	return members
}

func isPrimary(member map[string]any) bool {
	primary, _ := core.Object(member).Get("primary").(bool)
	return primary
}
